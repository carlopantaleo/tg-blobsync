package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"time"

	"tg-blobsync/internal/domain"
	"tg-blobsync/internal/pkg/retry"

	"github.com/gotd/td/crypto"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

// ListFiles returns files from the topic.
// History requests are automatically retried on FLOOD_WAIT errors.
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
		var messages []tg.MessageClass
		var history tg.MessagesMessagesClass
		err := retry.WithRetry(ctx, "ListFiles page", func() error {
			var innerErr error
			history, innerErr = t.api.MessagesGetReplies(ctx, &tg.MessagesGetRepliesRequest{
				Peer:     inputPeer,
				MsgID:    int(topicID),
				OffsetID: offsetID,
				Limit:    limit,
			})
			return innerErr
		}, 5, time.Second)
		if err != nil {
			return nil, err
		}

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
			file, ok := t.parseMessageToFile(msg, topicID)
			if ok {
				files = append(files, file)
			}
		}

		lastMsg := messages[len(messages)-1]
		if lastMsg.GetID() >= offsetID && offsetID != 0 {
			break
		}
		offsetID = lastMsg.GetID()
	}

	return groupChunkFiles(files), nil
}

func groupChunkFiles(files []domain.RemoteFile) []domain.RemoteFile {
	groups := make(map[string][]domain.RemoteFile)
	order := make([]string, 0)
	var result []domain.RemoteFile
	for _, file := range files {
		if file.Meta.Flags != domain.ChunkFlag {
			result = append(result, file)
			continue
		}
		key := file.Meta.Path
		if file.Meta.Checksum != "" {
			key += "\x00" + file.Meta.Checksum
		}
		if _, exists := groups[key]; !exists {
			order = append(order, key)
		}
		groups[key] = append(groups[key], file)
	}
	for _, key := range order {
		chunks := groups[key]
		sort.Slice(chunks, func(i, j int) bool { return chunks[i].Meta.Idx < chunks[j].Meta.Idx })
		if !isCompleteChunkSet(chunks) {
			continue
		}
		chunkIDs := make([]int, len(chunks))
		var totalSize int64
		for i, chunk := range chunks {
			chunkIDs[i] = chunk.MessageID
			totalSize += chunk.Size
		}
		first := chunks[0]
		first.ChunkIDs = chunkIDs
		first.Size = totalSize
		result = append(result, first)
	}
	return result
}

func isCompleteChunkSet(chunks []domain.RemoteFile) bool {
	if len(chunks) == 0 || chunks[0].Meta.Idx != 1 {
		return false
	}
	for index, chunk := range chunks {
		if chunk.Meta.Idx != index+1 {
			return false
		}
	}
	return true
}

// GroupTotals returns aggregate file count and size, using topic indexes when available.
func (t *TelegramClient) GroupTotals(ctx context.Context, groupID int64) (domain.GroupTotals, error) {
	topics, err := t.ListTopics(ctx, groupID)
	if err != nil {
		return domain.GroupTotals{}, err
	}
	var totals domain.GroupTotals
	for _, topic := range topics {
		index, _, indexed, err := t.GetIndex(ctx, groupID, topic.ID)
		if err != nil {
			return totals, err
		}
		if indexed {
			addIndexTotals(&totals, *index)
			continue
		}
		files, err := t.ListFiles(ctx, groupID, topic.ID)
		if err != nil {
			return totals, err
		}
		for _, file := range files {
			totals.Files++
			totals.TotalSize += file.Size
		}
	}
	return totals, nil
}

func (t *TelegramClient) parseMessageToFile(msg tg.MessageClass, topicID int64) (domain.RemoteFile, bool) {
	m, ok := msg.(*tg.Message)
	if !ok {
		return domain.RemoteFile{}, false
	}

	// Topic Filter Logic
	if topicID != 0 {
		match := false
		if m.ReplyTo != nil {
			if h, ok := m.ReplyTo.(*tg.MessageReplyHeader); ok {
				if h.ReplyToTopID == int(topicID) || h.ReplyToMsgID == int(topicID) {
					match = true
				}
			}
		}
		if !match {
			return domain.RemoteFile{}, false
		}
	}

	// Parse Caption and Document Info
	if m.Message != "" {
		var meta domain.FileMeta
		// Ignore unmarshal errors, it means it's not a file created by us
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
				if meta.Flags == domain.EmptyFileFlag {
					size = 0
				}
				return domain.RemoteFile{
					Meta:      meta,
					MessageID: m.ID,
					Size:      size,
				}, true
			}
		}
	}
	return domain.RemoteFile{}, false
}

func addIndexTotals(totals *domain.GroupTotals, index domain.FileIndex) {
	totals.Files += len(index.Entries)
	for _, entry := range index.Entries {
		totals.TotalSize += entry.Size
	}
}

// UploadFile uploads a file to the topic with progress reporting.
func (t *TelegramClient) UploadFile(ctx context.Context, groupID int64, topicID int64, file domain.LocalFile) error {
	if t.chunkThreshold > 0 && file.Size > t.chunkThreshold {
		_, err := t.UploadChunkedFile(ctx, groupID, topicID, file)
		return err
	}

	accessHash, _ := t.getAccessHash(groupID)
	inputPeer := &tg.InputPeerChannel{
		ChannelID:  groupID,
		AccessHash: accessHash,
	}

	log.Printf("[...] Uploading: %s (%s)", file.Path, formatSize(file.Size))

	var task domain.ProgressTask

	err := retry.WithRetry(ctx, "UploadFile: "+file.Path, func() error {
		// 0. Generate a fresh upload ID for each retry to ensure a clean state
		uploadID, _ := crypto.RandInt64(crypto.DefaultRand())

		t.mu.Lock()
		t.progressStarts[uploadID] = time.Now()
		if t.progressTracker != nil {
			// If we already had a task from a previous attempt, abort it before starting a new one
			if task != nil {
				task.Abort()
			}
			task = t.progressTracker.Start(file.Path, file.Size)
			t.progressTasks[uploadID] = task
		}
		t.mu.Unlock()

		// Ensure we clean up progress tracking for this ID if this attempt fails
		defer func() {
			t.mu.Lock()
			delete(t.progressStarts, uploadID)
			delete(t.progressTasks, uploadID)
			t.mu.Unlock()
		}()

		// 1. Raw content upload
		var u tg.InputFileClass
		var uploadErr error

		// Special case for empty files: Telegram rejects 0-byte files.
		// We upload a 1-byte dummy file and mark it with a flag.
		if file.Size == 0 {
			u, uploadErr = t.uploader.WithIDGenerator(func() (int64, error) {
				return uploadID, nil
			}).FromBytes(ctx, filepath.Base(file.Path), []byte{0})
		} else {
			// If it's a file from disk, use uploader.FromPath for potential optimizations (like random access for concurrent parts)
			u, uploadErr = t.uploader.WithIDGenerator(func() (int64, error) {
				return uploadID, nil
			}).FromPath(ctx, file.AbsPath)
		}

		if uploadErr != nil {
			return fmt.Errorf("failed to upload raw content: %w", uploadErr)
		}

		// 2. JSON Metadata preparation
		meta := domain.FileMeta{
			Path:     file.Path,
			Checksum: file.Checksum,
			ModTime:  file.ModTime,
		}
		if file.Size == 0 {
			meta.Flags = domain.EmptyFileFlag
		}
		captionBytes, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		caption := string(captionBytes)

		// 3. MIME type determination
		mimeType := mime.TypeByExtension(filepath.Ext(file.Path))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		// 4. Send Message with Document
		_, err = t.sender.To(inputPeer).
			Reply(int(topicID)).
			Media(ctx, message.UploadedDocument(u, styling.Plain(caption)).
				MIME(mimeType).
				Filename(filepath.Base(file.Path)),
			)

		if err != nil {
			return fmt.Errorf("failed to send document message: %w", err)
		}
		return nil
	}, 5, 1*time.Second)

	if err != nil {
		if task != nil {
			task.Abort()
		}
		return err
	}

	if task != nil {
		task.Complete()
	}
	log.Printf("[+] Uploaded: %s", file.Path)
	return nil
}

// UploadChunkedFile uploads a logical file as ordered Telegram document chunks.
func (t *TelegramClient) UploadChunkedFile(ctx context.Context, groupID, topicID int64, file domain.LocalFile) ([]int, error) {
	plan := chunkPlan(file.Size, t.chunkThreshold, t.chunkSize)
	if len(plan) == 0 {
		return nil, fmt.Errorf("invalid chunk plan for %s", file.Path)
	}

	inputPeer := &tg.InputPeerChannel{ChannelID: groupID, AccessHash: t.getCachedAccessHash(groupID)}
	fd, err := os.Open(file.AbsPath)
	if err != nil {
		return nil, fmt.Errorf("open file for chunking: %w", err)
	}
	defer fd.Close()

	var task domain.ProgressTask
	if t.progressTracker != nil {
		task = t.progressTracker.Start(file.Path, file.Size)
	}
	uploadedIDs := make([]int, 0, len(plan))
	var completed int64
	for index, chunk := range plan {
		if task != nil {
			task.SetChunk(index+1, len(plan))
		}
		// Pass index+1 to start Idx at 1 instead of 0
		messageID, uploadErr := t.uploadChunk(ctx, inputPeer, topicID, fd, file, chunk, index+1, completed, task)
		if uploadErr != nil {
			if task != nil {
				task.Abort()
			}
			for _, uploadedID := range uploadedIDs {
				if deleteErr := t.DeleteFile(ctx, groupID, topicID, uploadedID); deleteErr != nil {
					log.Printf("Warning: failed to clean up chunk %d: %v", uploadedID, deleteErr)
				}
			}
			return nil, uploadErr
		}
		uploadedIDs = append(uploadedIDs, messageID)
		completed += chunk.Length
	}
	if task != nil {
		task.Complete()
	}
	return uploadedIDs, nil
}

func (t *TelegramClient) uploadChunk(ctx context.Context, peer *tg.InputPeerChannel, topicID int64, file *os.File, local domain.LocalFile, chunk chunkRange, index int, base int64, task domain.ProgressTask) (int, error) {
	uploadID, err := crypto.RandInt64(crypto.DefaultRand())
	if err != nil {
		return 0, fmt.Errorf("generate upload ID: %w", err)
	}
	reader := io.NewSectionReader(file, chunk.Offset, chunk.Length)
	t.mu.Lock()
	if t.progressStarts == nil {
		t.progressStarts = make(map[int64]time.Time)
	}
	if t.progressTasks == nil {
		t.progressTasks = make(map[int64]domain.ProgressTask)
	}
	if t.progressBases == nil {
		t.progressBases = make(map[int64]int64)
	}
	t.progressStarts[uploadID] = time.Now()
	t.progressBases[uploadID] = base
	if task != nil {
		t.progressTasks[uploadID] = task
	}
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.progressStarts, uploadID)
		delete(t.progressTasks, uploadID)
		delete(t.progressBases, uploadID)
		t.mu.Unlock()
	}()

	u, err := t.uploader.WithIDGenerator(func() (int64, error) { return uploadID, nil }).FromReader(ctx, filepath.Base(local.Path), reader)
	if err != nil {
		return 0, fmt.Errorf("upload chunk %d: %w", index, err)
	}
	meta := domain.FileMeta{Path: local.Path, Checksum: local.Checksum, ModTime: local.ModTime, Flags: domain.ChunkFlag, Idx: index}
	captionBytes, err := json.Marshal(meta)
	if err != nil {
		return 0, fmt.Errorf("marshal chunk metadata: %w", err)
	}
	mimeType := mime.TypeByExtension(filepath.Ext(local.Path))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	sent, err := t.sender.To(peer).Reply(int(topicID)).Media(ctx, message.UploadedDocument(u, styling.Plain(string(captionBytes))).MIME(mimeType).Filename(filepath.Base(local.Path)))
	if err != nil {
		return 0, fmt.Errorf("send chunk %d: %w", index, err)
	}
	messageID, ok := messageIDFromUpdates(sent)
	if !ok {
		return 0, fmt.Errorf("chunk %d message ID not found", index)
	}
	return messageID, nil
}

func (t *TelegramClient) getCachedAccessHash(groupID int64) int64 {
	accessHash, _ := t.getAccessHash(groupID)
	return accessHash
}

// Chunk implements uploader.Progress interface.
func (t *TelegramClient) Chunk(ctx context.Context, state uploader.ProgressState) error {
	t.mu.RLock()
	task, hasTask := t.progressTasks[state.ID]
	startTime, hasStart := t.progressStarts[state.ID]
	base := t.progressBases[state.ID]
	t.mu.RUnlock()

	if hasTask {
		task.SetCurrent(base + state.Uploaded)
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
		if t.progressTracker == nil {
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

// DeleteChunkedFile deletes every Telegram message belonging to a logical file.
func (t *TelegramClient) DeleteChunkedFile(ctx context.Context, groupID, topicID int64, chunkIDs []int) error {
	for _, messageID := range chunkIDs {
		if err := t.DeleteFile(ctx, groupID, topicID, messageID); err != nil {
			return err
		}
	}
	return nil
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

// DownloadChunkedFile returns a lazy reader that concatenates chunk messages in order.
func (t *TelegramClient) DownloadChunkedFile(ctx context.Context, groupID, topicID int64, chunkIDs []int, fileName string, size int64) (io.ReadCloser, error) {
	return newChunkReader(chunkIDs, func(messageID int) (io.ReadCloser, error) {
		return t.DownloadFile(ctx, groupID, topicID, messageID, fileName, size)
	}), nil
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

	var msg *tg.Message
	{
		err := retry.WithRetry(ctx, "DownloadFile setup: "+fileName, func() error {
			msgs, err := t.api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
				Channel: &tg.InputChannel{
					ChannelID:  groupID,
					AccessHash: accessHash,
				},
				ID: []tg.InputMessageClass{&tg.InputMessageID{ID: messageID}},
			})
			if err != nil {
				return err
			}

			switch m := msgs.(type) {
			case *tg.MessagesChannelMessages:
				if len(m.Messages) > 0 {
					if mm, ok := m.Messages[0].(*tg.Message); ok {
						msg = mm
						return nil
					}
				}
			}
			return errors.New("message not found or invalid type")
		}, 5, 1*time.Second)

		if err != nil {
			t.mu.Lock()
			delete(t.progressStarts, downloadID)
			t.mu.Unlock()
			return nil, err
		}
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

	// Pipe for streaming
	pr, pw := io.Pipe()

	var task domain.ProgressTask
	if t.progressTracker != nil {
		task = t.progressTracker.Start(fileName, size)
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
	if tw.total <= 0 || tw.t.progressTracker != nil {
		return
	}

	// Log every 5MB or at the end
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
