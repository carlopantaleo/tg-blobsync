package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"tg-blobsync/internal/domain"
	"tg-blobsync/internal/pkg/retry"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"
)

const indexCaption = `{"f":"INDEX"}`

// GetIndex returns the topic index when it is stored in the most recent message.
func (t *TelegramClient) GetIndex(ctx context.Context, groupID int64, topicID int64) (*domain.FileIndex, int, bool, error) {
	message, err := t.getLatestTopicMessage(ctx, groupID, topicID)
	if err != nil {
		return nil, 0, false, err
	}
	if message == nil || !isIndexMessage(message) {
		return nil, 0, false, nil
	}

	document, ok := message.Media.(*tg.MessageMediaDocument)
	if !ok {
		return nil, 0, false, fmt.Errorf("index message %d does not contain a document", message.ID)
	}
	file, ok := document.Document.(*tg.Document)
	if !ok {
		return nil, 0, false, fmt.Errorf("index message %d has an invalid document", message.ID)
	}

	reader, err := t.DownloadFile(ctx, groupID, topicID, message.ID, "index.json", file.Size, nil)
	if err != nil {
		return nil, 0, false, fmt.Errorf("download topic index: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, 0, false, fmt.Errorf("read topic index: %w", err)
	}

	var index domain.FileIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, 0, false, fmt.Errorf("unmarshal topic index: %w", err)
	}
	return &index, message.ID, true, nil
}

// UploadIndex uploads index.json as the most recent message in a topic.
func (t *TelegramClient) UploadIndex(ctx context.Context, groupID int64, topicID int64, index domain.FileIndex) (int, error) {
	data, err := json.Marshal(index)
	if err != nil {
		return 0, fmt.Errorf("marshal topic index: %w", err)
	}

	file, err := t.uploader.FromBytes(ctx, "index.json", data)
	if err != nil {
		return 0, fmt.Errorf("upload topic index document: %w", err)
	}

	accessHash, _ := t.getAccessHash(groupID)
	peer := &tg.InputPeerChannel{ChannelID: groupID, AccessHash: accessHash}
	sent, err := t.sender.To(peer).Reply(int(topicID)).Media(ctx, message.UploadedDocument(file, styling.Plain(indexCaption)).MIME("application/json").Filename("index.json"))
	if err != nil {
		return 0, fmt.Errorf("send topic index message: %w", err)
	}
	messageID, ok := messageIDFromUpdates(sent)
	if !ok {
		return 0, fmt.Errorf("topic index message ID not found in Telegram updates")
	}
	return messageID, nil
}

func messageIDFromUpdates(updates tg.UpdatesClass) (int, bool) {
	var values []tg.UpdateClass
	switch update := updates.(type) {
	case *tg.Updates:
		values = update.Updates
	case *tg.UpdatesCombined:
		values = update.Updates
	case *tg.UpdateShort:
		values = []tg.UpdateClass{update.Update}
	}
	for _, value := range values {
		switch update := value.(type) {
		case *tg.UpdateNewChannelMessage:
			return update.Message.GetID(), true
		case *tg.UpdateNewMessage:
			return update.Message.GetID(), true
		}
	}
	return 0, false
}

// ListIndexMessageIDs returns all INDEX message IDs in a topic.
func (t *TelegramClient) ListIndexMessageIDs(ctx context.Context, groupID int64, topicID int64) ([]int, error) {
	accessHash, _ := t.getAccessHash(groupID)
	peer := &tg.InputPeerChannel{ChannelID: groupID, AccessHash: accessHash}
	var ids []int
	offsetID := 0

	for {
		var history tg.MessagesMessagesClass
		err := retry.WithRetry(ctx, "ListIndexMessageIDs page", func() error {
			var innerErr error
			history, innerErr = t.api.MessagesGetReplies(ctx, &tg.MessagesGetRepliesRequest{
				Peer: peer, MsgID: int(topicID), OffsetID: offsetID, Limit: 100,
			})
			return innerErr
		}, 5, time.Second)
		if err != nil {
			return nil, err
		}
		messages := replyMessages(history)
		if len(messages) == 0 {
			return ids, nil
		}
		for _, raw := range messages {
			if msg, ok := raw.(*tg.Message); ok && isIndexMessage(msg) {
				ids = append(ids, msg.ID)
			}
		}
		lastID := messages[len(messages)-1].GetID()
		if lastID >= offsetID && offsetID != 0 {
			return ids, nil
		}
		offsetID = lastID
	}
}

func (t *TelegramClient) getLatestTopicMessage(ctx context.Context, groupID int64, topicID int64) (*tg.Message, error) {
	accessHash, _ := t.getAccessHash(groupID)
	var history tg.MessagesMessagesClass
	err := retry.WithRetry(ctx, "getLatestTopicMessage", func() error {
		var innerErr error
		history, innerErr = t.api.MessagesGetReplies(ctx, &tg.MessagesGetRepliesRequest{
			Peer:  &tg.InputPeerChannel{ChannelID: groupID, AccessHash: accessHash},
			MsgID: int(topicID), Limit: 1,
		})
		return innerErr
	}, 5, time.Second)
	if err != nil {
		return nil, err
	}
	messages := replyMessages(history)
	if len(messages) == 0 {
		return nil, nil
	}
	message, _ := messages[0].(*tg.Message)
	return message, nil
}

func replyMessages(history tg.MessagesMessagesClass) []tg.MessageClass {
	switch messages := history.(type) {
	case *tg.MessagesChannelMessages:
		return messages.Messages
	case *tg.MessagesMessagesSlice:
		return messages.Messages
	case *tg.MessagesMessages:
		return messages.Messages
	default:
		return nil
	}
}

func isIndexMessage(message *tg.Message) bool {
	var meta domain.FileMeta
	return json.Unmarshal([]byte(message.Message), &meta) == nil && meta.Flags == domain.IndexFlag
}
