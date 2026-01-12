package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"tg-blobsync/internal/domain"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

// dummyTracker is a simple implementation of domain.ProgressTracker for testing.
type dummyTracker struct{}

func (d *dummyTracker) SetTotalFiles(total int) {}

func (d *dummyTracker) Start(name string, total int64) domain.ProgressTask {
	return nil
}

func (d *dummyTracker) Wait() {}

func TestTelegramClient_ListFiles(t *testing.T) {
	mockInvoker := NewMockInvoker()

	// Setup TelegramClient with mock api
	client := &TelegramClient{
		api:            tg.NewClient(mockInvoker),
		peerCache:      make(map[int64]int64),
		progressStarts: make(map[int64]time.Time),
		progressTasks:  make(map[int64]domain.ProgressTask),
	}
	client.sender = message.NewSender(client.api)
	client.setAccessHash(100, 12345) // Group ID 100

	// Define test data
	topicID := int64(200)

	// Prepare messages to return
	// We need to simulate:
	// 1. A message that is a file in the topic
	// 2. A message that is not a file
	// 3. A message that is a file but not in the topic (should be filtered out by logic or query?)
	// Note: ListFiles uses MessagesGetHistory with offset. We mock one page.

	fileMeta := domain.FileMeta{
		Path:     "test.txt",
		Checksum: "abc",
		ModTime:  time.Now().Unix(),
	}
	metaBytes, _ := json.Marshal(fileMeta)

	msg1 := &tg.Message{
		ID:      1,
		Date:    int(time.Now().Unix()),
		PeerID:  &tg.PeerChannel{ChannelID: 100},
		Message: string(metaBytes),
		ReplyTo: &tg.MessageReplyHeader{
			ReplyToTopID: int(topicID),
		},
		Media: &tg.MessageMediaDocument{
			Document: &tg.Document{
				ID:            11,
				AccessHash:    11,
				FileReference: []byte{1},
				Date:          int(time.Now().Unix()),
				MimeType:      "text/plain",
				Size:          1024,
				DCID:          1,
			},
		},
	}

	// Mock response
	mockInvoker.Register(&tg.MessagesGetHistoryRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		// Verify input if needed
		req := input.(*tg.MessagesGetHistoryRequest)
		if req.Peer == nil {
			return errors.New("missing peer")
		}

		if req.OffsetID > 0 {
			resp := &tg.MessagesChannelMessages{
				Messages: []tg.MessageClass{},
				Count:    1,
				Chats:    []tg.ChatClass{},
				Users:    []tg.UserClass{},
			}
			buf := new(bin.Buffer)
			if err := resp.Encode(buf); err != nil {
				return err
			}
			return output.Decode(buf)
		}

		// Return MessagesChannelMessages
		resp := &tg.MessagesChannelMessages{
			Messages: []tg.MessageClass{msg1},
			Count:    1,
			Chats:    []tg.ChatClass{},
			Users:    []tg.UserClass{},
		}

		buf := new(bin.Buffer)
		if err := resp.Encode(buf); err != nil {
			return err
		}

		return output.Decode(buf)
	})

	files, err := client.ListFiles(context.Background(), 100, topicID)
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(files))
	}
	if files[0].Meta.Path != "test.txt" {
		t.Errorf("Unexpected file path: %s", files[0].Meta.Path)
	}
}

func TestTelegramClient_DeleteFile(t *testing.T) {
	mockInvoker := NewMockInvoker()

	client := &TelegramClient{
		api:            tg.NewClient(mockInvoker),
		peerCache:      make(map[int64]int64),
		progressStarts: make(map[int64]time.Time),
		progressTasks:  make(map[int64]domain.ProgressTask),
	}
	client.sender = message.NewSender(client.api)
	client.setAccessHash(100, 12345)

	mockInvoker.Register(&tg.ChannelsDeleteMessagesRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		req := input.(*tg.ChannelsDeleteMessagesRequest)
		if len(req.ID) != 1 || req.ID[0] != 123 {
			return errors.New("unexpected message ID")
		}

		// ChannelsDeleteMessages typically returns MessagesAffectedMessages in some schema versions
		// or Updates in others. The error "unable to decode messages.affectedMessages" implies
		// the client expects messages.affectedMessages.
		resp := &tg.MessagesAffectedMessages{
			Pts:      1,
			PtsCount: 1,
		}
		buf := new(bin.Buffer)
		if err := resp.Encode(buf); err != nil {
			return err
		}
		return output.Decode(buf)
	})

	err := client.DeleteFile(context.Background(), 100, 200, 123)
	if err != nil {
		t.Errorf("DeleteFile failed: %v", err)
	}
}

func TestTelegramClient_ResolveGroup_Success(t *testing.T) {
	mockInvoker := NewMockInvoker()

	client := &TelegramClient{
		api:            tg.NewClient(mockInvoker),
		peerCache:      make(map[int64]int64),
		progressStarts: make(map[int64]time.Time),
		progressTasks:  make(map[int64]domain.ProgressTask),
	}
	client.sender = message.NewSender(client.api)
	// Initial cache empty

	mockInvoker.Register(&tg.MessagesGetDialogsRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		resp := &tg.MessagesDialogs{
			Chats: []tg.ChatClass{
				&tg.Channel{
					ID:         100,
					Title:      "MyGroup",
					Megagroup:  true,
					AccessHash: 999,
					Date:       int(time.Now().Unix()),
					Photo:      &tg.ChatPhotoEmpty{},
				},
			},
			Messages: []tg.MessageClass{},
			Users:    []tg.UserClass{},
		}
		buf := new(bin.Buffer)
		if err := resp.Encode(buf); err != nil {
			return err
		}
		return output.Decode(buf)
	})

	err := client.ResolveGroup(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveGroup failed: %v", err)
	}

	// Check cache
	if hash, ok := client.getAccessHash(100); !ok || hash != 999 {
		t.Errorf("AccessHash not cached correctly")
	}
}

func TestTelegramClient_ResolveGroup_NotFound(t *testing.T) {
	mockInvoker := NewMockInvoker()

	client := &TelegramClient{
		api:            tg.NewClient(mockInvoker),
		peerCache:      make(map[int64]int64),
		progressStarts: make(map[int64]time.Time),
		progressTasks:  make(map[int64]domain.ProgressTask),
	}
	client.sender = message.NewSender(client.api)

	mockInvoker.Register(&tg.MessagesGetDialogsRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		resp := &tg.MessagesDialogs{
			Chats:    []tg.ChatClass{},
			Messages: []tg.MessageClass{},
			Users:    []tg.UserClass{},
		}
		buf := new(bin.Buffer)
		if err := resp.Encode(buf); err != nil {
			return err
		}
		return output.Decode(buf)
	})

	err := client.ResolveGroup(context.Background(), 100)
	if err == nil {
		t.Error("Expected error for missing group")
	}
}

func TestTelegramClient_ListTopics(t *testing.T) {
	mockInvoker := NewMockInvoker()

	client := &TelegramClient{
		api:            tg.NewClient(mockInvoker),
		peerCache:      make(map[int64]int64),
		progressStarts: make(map[int64]time.Time),
		progressTasks:  make(map[int64]domain.ProgressTask),
	}
	client.sender = message.NewSender(client.api)
	client.setAccessHash(100, 12345)

	mockInvoker.Register(&tg.MessagesGetForumTopicsRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		resp := &tg.MessagesForumTopics{
			Topics: []tg.ForumTopicClass{
				&tg.ForumTopic{
					ID:    1,
					Title: "General",
					Date:  int(time.Now().Unix()),
					// Some versions require FromID or other fields
					// We'll add FromID just in case
					FromID: &tg.PeerUser{UserID: 1},
					Peer:   &tg.PeerChannel{ChannelID: 100},
				},
				&tg.ForumTopic{
					ID:     2,
					Title:  "Random",
					Date:   int(time.Now().Unix()),
					FromID: &tg.PeerUser{UserID: 1},
					Peer:   &tg.PeerChannel{ChannelID: 100},
				},
			},
			Messages: []tg.MessageClass{},
			Chats:    []tg.ChatClass{},
			Users:    []tg.UserClass{},
			Count:    2,
		}
		buf := new(bin.Buffer)
		if err := resp.Encode(buf); err != nil {
			return err
		}
		return output.Decode(buf)
	})

	topics, err := client.ListTopics(context.Background(), 100)
	if err != nil {
		t.Fatalf("ListTopics failed: %v", err)
	}
	if len(topics) != 2 {
		t.Errorf("Expected 2 topics, got %d", len(topics))
	}
}

func TestTelegramClient_SetUploadThreads(t *testing.T) {
	mockInvoker := NewMockInvoker()

	client := &TelegramClient{
		api:            tg.NewClient(mockInvoker),
		peerCache:      make(map[int64]int64),
		progressStarts: make(map[int64]time.Time),
		progressTasks:  make(map[int64]domain.ProgressTask),
		uploadThreads:  4, // default
	}

	// Test setting valid threads
	client.SetUploadThreads(8)
	if client.uploadThreads != 8 {
		t.Errorf("expected uploadThreads 8, got %d", client.uploadThreads)
	}

	// Test setting invalid threads (should default to 1)
	client.SetUploadThreads(0)
	if client.uploadThreads != 1 {
		t.Errorf("expected uploadThreads 1 for invalid input, got %d", client.uploadThreads)
	}

	client.SetUploadThreads(-1)
	if client.uploadThreads != 1 {
		t.Errorf("expected uploadThreads 1 for negative input, got %d", client.uploadThreads)
	}
}

func TestTelegramClient_SetProgressTracker(t *testing.T) {
	mockInvoker := NewMockInvoker()

	client := &TelegramClient{
		api:            tg.NewClient(mockInvoker),
		peerCache:      make(map[int64]int64),
		progressStarts: make(map[int64]time.Time),
		progressTasks:  make(map[int64]domain.ProgressTask),
	}

	// Simple implementation of ProgressTracker
	// type dummyTracker struct{}

	// func (d *dummyTracker) SetTotalFiles(total int) {}

	// func (d *dummyTracker) Start(name string, total int64) domain.ProgressTask {
	// 	return nil
	// }

	// func (d *dummyTracker) Wait() {}

	tracker := &dummyTracker{}

	client.SetProgressTracker(tracker)
	if client.progressTracker == nil {
		t.Error("expected progressTracker to be set")
	}

	// Test that the tracker is used (though we can't fully test without uploader)
	// This at least ensures the setter works
}

func TestTelegramClient_AccessHash(t *testing.T) {
	mockInvoker := NewMockInvoker()

	client := &TelegramClient{
		api:            tg.NewClient(mockInvoker),
		peerCache:      make(map[int64]int64),
		progressStarts: make(map[int64]time.Time),
		progressTasks:  make(map[int64]domain.ProgressTask),
	}

	// Initially not present
	_, ok := client.getAccessHash(100)
	if ok {
		t.Error("expected no access hash initially")
	}

	// Set and get
	client.setAccessHash(100, 999)
	hash, ok := client.getAccessHash(100)
	if !ok || hash != 999 {
		t.Errorf("expected access hash 999, got %d, ok %v", hash, ok)
	}
}
