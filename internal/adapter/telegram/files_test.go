package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

func TestTelegramClient_UploadChunk_Uses1BasedIndex(t *testing.T) {
	// A basic test to assert that logic starting `Idx` from 1 rather than 0 behaves correctly.
	// Since uploadChunk logic is mostly internal/hard to mock simply, we'll verify our assumption
	// on how the JSON marshals 1 vs 0 when `omitempty` is present, which is the core reason for the change.

	// First chunk (1-based index)
	metaFirst := domain.FileMeta{
		Path:  "chunked.bin",
		Idx:   1,
		Flags: domain.ChunkFlag,
	}

	bytesFirst, err := json.Marshal(metaFirst)
	if err != nil {
		t.Fatalf("Failed to marshal first chunk meta: %v", err)
	}

	strFirst := string(bytesFirst)
	if !strings.Contains(strFirst, `"i":1`) {
		t.Errorf("Expected first chunk to explicitly contain \"i\":1, got: %s", strFirst)
	}

	// Non-chunked file (index 0)
	metaNonChunked := domain.FileMeta{
		Path: "small.txt",
	}

	bytesNonChunked, err := json.Marshal(metaNonChunked)
	if err != nil {
		t.Fatalf("Failed to marshal non-chunked meta: %v", err)
	}

	strNonChunked := string(bytesNonChunked)
	if strings.Contains(strNonChunked, `"i":`) {
		t.Errorf("Expected non-chunked file to omit index completely, got: %s", strNonChunked)
	}
}

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

	// Mock MessagesGetRepliesRequest
	mockInvoker.Register(&tg.MessagesGetRepliesRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		// Verify input if needed
		req := input.(*tg.MessagesGetRepliesRequest)
		if req.Peer == nil {
			return errors.New("missing peer")
		}
		if req.MsgID != int(topicID) {
			return errors.New("unexpected topic ID")
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
	hash, ok := client.getAccessHash(100)
	if ok {
		t.Error("expected no access hash initially")
	}

	client.setAccessHash(100, 999)
	hash, ok = client.getAccessHash(100)
	if !ok || hash != 999 {
		t.Errorf("expected access hash 999, got %d, ok %v", hash, ok)
	}
}

func TestTelegramClient_ListFiles_Pagination(t *testing.T) {
	mockInvoker := NewMockInvoker()

	client := &TelegramClient{
		api:            tg.NewClient(mockInvoker),
		peerCache:      make(map[int64]int64),
		progressStarts: make(map[int64]time.Time),
		progressTasks:  make(map[int64]domain.ProgressTask),
	}
	client.invoker = mockInvoker
	client.sender = message.NewSender(client.api)
	client.setAccessHash(100, 12345) // Group ID 100

	topicID := int64(200)

	fileMeta := domain.FileMeta{
		Path:     "test_pagination.txt",
		Checksum: "pag_abc",
		ModTime:  time.Now().Unix(),
	}
	metaBytes, _ := json.Marshal(fileMeta)

	fileMsg := &tg.Message{
		ID:      3001,
		Date:    int(time.Now().Unix()),
		PeerID:  &tg.PeerChannel{ChannelID: 100},
		Message: string(metaBytes),
		ReplyTo: &tg.MessageReplyHeader{
			ReplyToTopID: int(topicID),
		},
		Media: &tg.MessageMediaDocument{
			Document: &tg.Document{
				ID:            12,
				AccessHash:    12,
				FileReference: []byte{2},
				Date:          int(time.Now().Unix()),
				MimeType:      "text/plain",
				Size:          2048,
			},
		},
	}

	messagesGetHistoryCallCount := 0

	mockInvoker.Register(&tg.MessagesGetRepliesRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		messagesGetHistoryCallCount++
		if messagesGetHistoryCallCount == 1 {
			// Return a high count to show pagination is used even for large topics
			resp := &tg.MessagesChannelMessages{
				Messages: []tg.MessageClass{fileMsg},
				Count:    3001,
			}
			return encodeResponse(output, resp)
		}
		// Second call returns empty list to stop pagination
		resp := &tg.MessagesChannelMessages{
			Messages: []tg.MessageClass{},
			Count:    0,
		}
		return encodeResponse(output, resp)
	})

	files, err := client.ListFiles(context.Background(), 100, topicID)
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(files))
	}
	if files[0].Meta.Path != "test_pagination.txt" {
		t.Errorf("Unexpected file path: %s", files[0].Meta.Path)
	}
}
