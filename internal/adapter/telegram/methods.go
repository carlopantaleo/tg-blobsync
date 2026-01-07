package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"path/filepath"

	"tg-blobsync/internal/domain"

	"time"

	"github.com/gotd/td/crypto"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

// ListGroups returns a list of Supergroups.
func (t *TelegramClient) ListGroups(ctx context.Context) ([]domain.Group, error) {
	dialogs, err := t.api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		Limit:      100,
		OffsetPeer: &tg.InputPeerEmpty{},
	})
	if err != nil {
		return nil, err
	}

	var groups []domain.Group
	var chats []tg.ChatClass

	switch d := dialogs.(type) {
	case *tg.MessagesDialogs:
		chats = d.Chats
	case *tg.MessagesDialogsSlice:
		chats = d.Chats
	}

	for _, chat := range chats {
		switch c := chat.(type) {
		case *tg.Channel:
			if c.Megagroup {
				t.setAccessHash(c.ID, c.AccessHash)
				groups = append(groups, domain.Group{
					ID:    c.ID,
					Title: c.Title,
				})
			}
		}
	}

	return groups, nil
}

// ResolveGroup ensures the AccessHash for the given groupID is cached.
func (t *TelegramClient) ResolveGroup(ctx context.Context, groupID int64) error {
	if _, ok := t.getAccessHash(groupID); ok {
		return nil
	}
	_, err := t.ListGroups(ctx)
	if err != nil {
		return err
	}
	if _, ok := t.getAccessHash(groupID); ok {
		return nil
	}
	return fmt.Errorf("group %d not found in recent dialogs", groupID)
}

// ListTopics returns a list of Forum Topics in a Supergroup.
func (t *TelegramClient) ListTopics(ctx context.Context, groupID int64) ([]domain.Topic, error) {
	accessHash, _ := t.getAccessHash(groupID)
	inputPeer := &tg.InputPeerChannel{
		ChannelID:  groupID,
		AccessHash: accessHash,
	}

	res, err := t.api.MessagesGetForumTopics(ctx, &tg.MessagesGetForumTopicsRequest{
		Peer:  inputPeer,
		Limit: 100,
	})
	if err != nil {
		return nil, err
	}

	var topics []domain.Topic
	for _, topic := range res.Topics {
		switch t := topic.(type) {
		case *tg.ForumTopic:
			topics = append(topics, domain.Topic{
				ID:    int64(t.ID),
				Title: t.Title,
			})
		}
	}

	return topics, nil
}

// ListFiles returns files from the topic.
func (t *TelegramClient) ListFiles(ctx context.Context, groupID int64, topicID int64) ([]domain.RemoteFile, error) {
	accessHash, _ := t.getAccessHash(groupID)
	inputPeer := &tg.InputPeerChannel{
		ChannelID:  groupID,
		AccessHash: accessHash,
	}

	var files []domain.RemoteFile
	offsetID := 0
	limit := 100

	for {
		history, err := t.api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:     inputPeer,
			OffsetID: offsetID,
			Limit:    limit,
		})
		if err != nil {
			return nil, err
		}

		var messages []tg.MessageClass
		switch h := history.(type) {
		case *tg.MessagesChannelMessages:
			messages = h.Messages
		case *tg.MessagesMessagesSlice:
			messages = h.Messages
		case *tg.MessagesMessages:
			messages = h.Messages
		}

		if len(messages) == 0 {
			break
		}

		for _, msg := range messages {
			m, ok := msg.(*tg.Message)
			if !ok {
				continue
			}

			// Topic Filter Logic
			topicMatch := false
			if topicID == 0 {
				topicMatch = true
			} else {
				if m.ReplyTo != nil {
					if h, ok := m.ReplyTo.(*tg.MessageReplyHeader); ok {
						if h.ReplyToTopID == int(topicID) || h.ReplyToMsgID == int(topicID) {
							topicMatch = true
						}
					}
				}
			}

			if !topicMatch {
				continue
			}

			// Parse Caption and Document Info
			if m.Message != "" {
				var meta domain.FileMeta
				// Ignoriamo errori di unmarshal, significa che non è un file nostro
				if err := json.Unmarshal([]byte(m.Message), &meta); err == nil {
					if meta.Path != "" && (meta.Checksum != "" || meta.ModTime != 0) {
						size := int64(0)
						if m.Media != nil {
							if doc, ok := m.Media.(*tg.MessageMediaDocument); ok {
								if d, ok := doc.Document.(*tg.Document); ok {
									size = d.Size
								}
							}
						}
						files = append(files, domain.RemoteFile{
							Meta:      meta,
							MessageID: m.ID,
							Size:      size,
						})
					}
				}
			}
		}

		lastMsg := messages[len(messages)-1]
		if lastMsg.GetID() >= offsetID && offsetID != 0 {
			break
		}
		offsetID = lastMsg.GetID()
	}

	return files, nil
}

// UploadFile uploads a file to the topic with progress reporting.
func (t *TelegramClient) UploadFile(ctx context.Context, groupID int64, topicID int64, file domain.LocalFile) error {
	accessHash, _ := t.getAccessHash(groupID)
	inputPeer := &tg.InputPeerChannel{
		ChannelID:  groupID,
		AccessHash: accessHash,
	}

	log.Printf("[...] Uploading: %s (%s)", file.Path, formatSize(file.Size))

	// Track start time for speed calculation and create progress task
	uploadID, _ := crypto.RandInt64(crypto.DefaultRand())
	t.mu.Lock()
	t.progressStarts[uploadID] = time.Now()
	if t.progressReporter != nil {
		t.progressTasks[uploadID] = t.progressReporter.Start(file.Path, file.Size)
	}
	t.mu.Unlock()

	var uploadSuccess bool
	defer func() {
		t.mu.Lock()
		delete(t.progressStarts, uploadID)
		if task, ok := t.progressTasks[uploadID]; ok {
			if uploadSuccess {
				task.Complete()
			} else {
				task.Abort()
			}
			delete(t.progressTasks, uploadID)
		}
		t.mu.Unlock()
	}()

	// 1. Upload del contenuto grezzo
	var u tg.InputFileClass
	var uploadErr error

	if file.Size == 0 {
		// Special case for empty files to avoid FILE_PARTS_INVALID
		u, uploadErr = t.uploader.WithIDGenerator(func() (int64, error) {
			return uploadID, nil
		}).FromBytes(ctx, filepath.Base(file.Path), []byte{})
	} else {
		// If it's a file from disk, use uploader.FromPath for potential optimizations (like random access for concurrent parts)
		u, uploadErr = t.uploader.WithIDGenerator(func() (int64, error) {
			return uploadID, nil
		}).FromPath(ctx, file.AbsPath)
	}

	if uploadErr != nil {
		return fmt.Errorf("failed to upload raw content: %w", uploadErr)
	}

	// 2. Preparazione Metadati JSON
	meta := domain.FileMeta{
		Path:     file.Path,
		Checksum: file.Checksum,
		ModTime:  file.ModTime,
	}
	captionBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	caption := string(captionBytes)

	// 3. Determinazione MIME type
	mimeType := mime.TypeByExtension(filepath.Ext(file.Path))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// 4. Invio Messaggio con Documento
	_, err = t.sender.To(inputPeer).
		Reply(int(topicID)).
		Media(ctx, message.UploadedDocument(u, styling.Plain(caption)).
			MIME(mimeType).
			Filename(filepath.Base(file.Path)),
		)

	if err != nil {
		return fmt.Errorf("failed to send document message: %w", err)
	}

	uploadSuccess = true
	log.Printf("[+] Uploaded: %s", file.Path)
	return nil
}

// Chunk implements uploader.Progress interface.
func (t *TelegramClient) Chunk(ctx context.Context, state uploader.ProgressState) error {
	t.mu.RLock()
	task, hasTask := t.progressTasks[state.ID]
	startTime, hasStart := t.progressStarts[state.ID]
	t.mu.RUnlock()

	if hasTask {
		task.SetCurrent(state.Uploaded)
	}

	if state.Total > 0 {
		percent := float64(state.Uploaded) / float64(state.Total) * 100

		speedStr := ""
		if hasStart {
			elapsed := time.Since(startTime).Seconds()
			if elapsed > 0 {
				speed := float64(state.Uploaded) / elapsed
				speedStr = fmt.Sprintf(" | %s/s", formatSize(int64(speed)))
			}
		}

		// Log only if no interactive reporter is active
		if t.progressReporter == nil {
			if state.Uploaded == state.Total || state.Uploaded%(5*1024*1024) < int64(state.PartSize) {
				log.Printf("  [%s] %.1f%% (%s/%s)%s", state.Name, percent, formatSize(state.Uploaded), formatSize(state.Total), speedStr)
			}
		}
	}
	return nil
}

func formatSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func (t *TelegramClient) DeleteFile(ctx context.Context, groupID int64, topicID int64, messageID int) error {
	accessHash, _ := t.getAccessHash(groupID)
	inputChannel := &tg.InputChannel{
		ChannelID:  groupID,
		AccessHash: accessHash,
	}

	_, err := t.api.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
		Channel: inputChannel,
		ID:      []int{messageID},
	})
	return err
}

func (t *TelegramClient) DownloadFile(ctx context.Context, groupID int64, topicID int64, messageID int, fileName string, size int64) (io.ReadCloser, error) {
	accessHash, _ := t.getAccessHash(groupID)

	log.Printf("[...] Downloading: %s (%s)", fileName, formatSize(size))

	// Track start time for speed calculation (using a negative ID for downloads to avoid collision with uploads if any)
	// Actually we can use the messageID as part of the key
	downloadID := int64(messageID)
	t.mu.Lock()
	t.progressStarts[downloadID] = time.Now()
	t.mu.Unlock()

	// Fetch del messaggio per ottenere la location del file
	msgs, err := t.api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: &tg.InputChannel{
			ChannelID:  groupID,
			AccessHash: accessHash,
		},
		ID: []tg.InputMessageClass{&tg.InputMessageID{ID: messageID}},
	})
	if err != nil {
		t.mu.Lock()
		delete(t.progressStarts, downloadID)
		t.mu.Unlock()
		return nil, err
	}

	var msg *tg.Message
	switch m := msgs.(type) {
	case *tg.MessagesChannelMessages:
		if len(m.Messages) > 0 {
			if mm, ok := m.Messages[0].(*tg.Message); ok {
				msg = mm
			}
		}
	}

	if msg == nil {
		t.mu.Lock()
		delete(t.progressStarts, downloadID)
		t.mu.Unlock()
		return nil, errors.New("message not found")
	}

	doc, ok := msg.Media.(*tg.MessageMediaDocument)
	if !ok {
		t.mu.Lock()
		delete(t.progressStarts, downloadID)
		t.mu.Unlock()
		return nil, errors.New("message is not a document")
	}

	d, ok := doc.Document.(*tg.Document)
	if !ok {
		t.mu.Lock()
		delete(t.progressStarts, downloadID)
		t.mu.Unlock()
		return nil, errors.New("media is not a document")
	}

	// Pipe per lo streaming
	pr, pw := io.Pipe()

	var task domain.ProgressTask
	if t.progressReporter != nil {
		task = t.progressReporter.Start(fileName, size)
	}

	var downloadSuccess bool
	go func() {
		defer func() {
			t.mu.Lock()
			delete(t.progressStarts, downloadID)
			t.mu.Unlock()
			if task != nil {
				if downloadSuccess {
					task.Complete()
				} else {
					task.Abort()
				}
			}
		}()

		// Create a custom writer that tracks progress and writes to pw
		tr := &trackingWriter{
			w:         pw,
			t:         t,
			id:        downloadID,
			name:      fileName,
			total:     size,
			lastLog:   0,
			startTime: time.Now(),
			task:      task,
		}

		// gotd downloader
		dl := downloader.NewDownloader().
			WithPartSize(512 * 1024) // Max part size for download
		// Check location
		loc := d.AsInputDocumentFileLocation()

		_, err := dl.Download(t.api, loc).Stream(ctx, tr)
		if err != nil {
			pw.CloseWithError(err)
		} else {
			downloadSuccess = true
			log.Printf("[+] Downloaded: %s", fileName)
			pw.Close()
		}
	}()

	return pr, nil
}

type trackingWriter struct {
	w         io.Writer
	t         *TelegramClient
	id        int64
	name      string
	total     int64
	uploaded  int64
	lastLog   int64
	startTime time.Time
	task      domain.ProgressTask
}

func (tw *trackingWriter) Write(p []byte) (n int, err error) {
	n, err = tw.w.Write(p)
	if n > 0 {
		tw.uploaded += int64(n)
		if tw.task != nil {
			tw.task.Increment(n)
		}
		tw.report()
	}
	return n, err
}

func (tw *trackingWriter) report() {
	if tw.total <= 0 || tw.t.progressReporter != nil {
		return
	}

	// Log ogni 5MB o alla fine
	if tw.uploaded == tw.total || tw.uploaded-tw.lastLog >= 5*1024*1024 {
		tw.lastLog = tw.uploaded
		percent := float64(tw.uploaded) / float64(tw.total) * 100
		elapsed := time.Since(tw.startTime).Seconds()
		speedStr := ""
		if elapsed > 0 {
			speed := float64(tw.uploaded) / elapsed
			speedStr = fmt.Sprintf(" | %s/s", formatSize(int64(speed)))
		}
		log.Printf("  [%s] %.1f%% (%s/%s)%s", tw.name, percent, formatSize(tw.uploaded), formatSize(tw.total), speedStr)
	}
}
