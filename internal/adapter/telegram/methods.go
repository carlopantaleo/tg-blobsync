package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"path/filepath"

	"tg-blobsync/internal/domain"

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

			// Parse Caption
			if m.Message != "" {
				var meta domain.FileMeta
				// Ignoriamo errori di unmarshal, significa che non è un file nostro
				if err := json.Unmarshal([]byte(m.Message), &meta); err == nil {
					if meta.Path != "" && meta.Checksum != "" {
						files = append(files, domain.RemoteFile{
							Meta:      meta,
							MessageID: m.ID,
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

func (t *TelegramClient) UploadFile(ctx context.Context, groupID int64, topicID int64, file domain.LocalFile, data io.Reader) error {
	accessHash, _ := t.getAccessHash(groupID)
	inputPeer := &tg.InputPeerChannel{
		ChannelID:  groupID,
		AccessHash: accessHash,
	}

	// 1. Upload del contenuto grezzo
	u, err := t.uploader.Upload(ctx, uploader.NewUpload(file.Path, data, file.Size))
	if err != nil {
		return err
	}

	// 2. Preparazione Metadati JSON
	meta := domain.FileMeta{
		Path:     file.Path,
		Checksum: file.Checksum,
	}
	captionBytes, err := json.Marshal(meta)
	if err != nil {
		return err
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

	return err
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

func (t *TelegramClient) DownloadFile(ctx context.Context, groupID int64, topicID int64, messageID int) (io.ReadCloser, error) {
	accessHash, _ := t.getAccessHash(groupID)

	// Fetch del messaggio per ottenere la location del file
	msgs, err := t.api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: &tg.InputChannel{
			ChannelID:  groupID,
			AccessHash: accessHash,
		},
		ID: []tg.InputMessageClass{&tg.InputMessageID{ID: messageID}},
	})
	if err != nil {
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
		return nil, errors.New("message not found")
	}

	doc, ok := msg.Media.(*tg.MessageMediaDocument)
	if !ok {
		return nil, errors.New("message is not a document")
	}

	d, ok := doc.Document.(*tg.Document)
	if !ok {
		return nil, errors.New("media is not a document")
	}

	// Pipe per lo streaming
	pr, pw := io.Pipe()

	go func() {
		// Create a custom writer that writes to pw
		// gotd downloader
		dl := downloader.NewDownloader()
		// Check location
		loc := d.AsInputDocumentFileLocation()

		_, err := dl.Download(t.api, loc).Stream(ctx, pw)
		if err != nil {
			pw.CloseWithError(err)
		} else {
			pw.Close()
		}
	}()

	return pr, nil
}
