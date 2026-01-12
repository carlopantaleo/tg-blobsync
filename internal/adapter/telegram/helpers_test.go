package telegram

import (
	"testing"

	"tg-blobsync/internal/domain"

	"github.com/gotd/td/tg"
)

// TestParseMessageToFile tests the private method parseMessageToFile by using a wrapper or reflection,
// or by exporting it for test (internal package allows access if tests are in same package).
// Since we are in package telegram, we can access private methods.

func TestParseMessageToFile(t *testing.T) {
	client := &TelegramClient{} // No need for full initialization for this method

	// now := int(time.Now().Unix())

	tests := []struct {
		name     string
		msg      tg.MessageClass
		topicID  int64
		wantFile bool
		wantPath string
		wantSize int64
	}{
		{
			name: "Valid file message",
			msg: &tg.Message{
				ID:      1,
				Message: `{"p":"folder/file.txt","m":"checksum","t":1234567890}`,
				Media: &tg.MessageMediaDocument{
					Document: &tg.Document{
						Size: 1024,
					},
				},
			},
			topicID:  0, // No topic filter
			wantFile: true,
			wantPath: "folder/file.txt",
			wantSize: 1024,
		},
		{
			name: "Valid empty file message",
			msg: &tg.Message{
				ID:      2,
				Message: `{"p":"folder/empty.txt","f":"EMPTY_FILE","t":1234567890}`,
			},
			topicID:  0,
			wantFile: true,
			wantPath: "folder/empty.txt",
			wantSize: 0,
		},
		{
			name: "Invalid json",
			msg: &tg.Message{
				ID:      3,
				Message: `not a json`,
			},
			topicID:  0,
			wantFile: false,
		},
		{
			name: "Missing path in json",
			msg: &tg.Message{
				ID:      4,
				Message: `{"m":"checksum"}`,
			},
			topicID:  0,
			wantFile: false,
		},
		{
			name: "Topic filter match (ReplyToMsgID)",
			msg: &tg.Message{
				ID:      5,
				Message: `{"p":"topic.txt","m":"c","t":1}`,
				ReplyTo: &tg.MessageReplyHeader{
					ReplyToMsgID: 100,
				},
				Media: &tg.MessageMediaDocument{
					Document: &tg.Document{Size: 100},
				},
			},
			topicID:  100,
			wantFile: true,
			wantPath: "topic.txt",
			wantSize: 100,
		},
		{
			name: "Topic filter match (ReplyToTopID)",
			msg: &tg.Message{
				ID:      6,
				Message: `{"p":"topic.txt","m":"c","t":1}`,
				ReplyTo: &tg.MessageReplyHeader{
					ReplyToTopID: 100,
				},
				Media: &tg.MessageMediaDocument{
					Document: &tg.Document{Size: 100},
				},
			},
			topicID:  100,
			wantFile: true,
			wantPath: "topic.txt",
			wantSize: 100,
		},
		{
			name: "Topic filter mismatch",
			msg: &tg.Message{
				ID:      7,
				Message: `{"p":"topic.txt","m":"c","t":1}`,
				ReplyTo: &tg.MessageReplyHeader{
					ReplyToMsgID: 999,
				},
				Media: &tg.MessageMediaDocument{
					Document: &tg.Document{Size: 100},
				},
			},
			topicID:  100,
			wantFile: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := client.parseMessageToFile(tt.msg, tt.topicID)
			if ok != tt.wantFile {
				t.Errorf("parseMessageToFile() ok = %v, want %v", ok, tt.wantFile)
				return
			}
			if tt.wantFile {
				if got.Meta.Path != tt.wantPath {
					t.Errorf("parseMessageToFile() path = %v, want %v", got.Meta.Path, tt.wantPath)
				}
				if got.Size != tt.wantSize {
					t.Errorf("parseMessageToFile() size = %v, want %v", got.Size, tt.wantSize)
				}
				if got.MessageID != tt.msg.GetID() {
					t.Errorf("parseMessageToFile() id = %v, want %v", got.MessageID, tt.msg.GetID())
				}
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
	}
	for _, tt := range tests {
		res := formatSize(tt.input)
		if res != tt.expected {
			t.Errorf("formatSize(%d) = %s, want %s", tt.input, res, tt.expected)
		}
	}
}

func TestSetUploadThreads(t *testing.T) {
	client := &TelegramClient{}
	client.SetUploadThreads(5)
	if client.uploadThreads != 5 {
		t.Errorf("expected 5 threads, got %d", client.uploadThreads)
	}
	client.SetUploadThreads(-1)
	if client.uploadThreads != 1 {
		t.Errorf("expected 1 thread for negative input, got %d", client.uploadThreads)
	}
}

func TestSetProgressTracker(t *testing.T) {
	client := &TelegramClient{}
	tracker := &mockTracker{}
	client.SetProgressTracker(tracker)
	if client.progressTracker != tracker {
		t.Error("SetProgressTracker failed")
	}
}

type mockTracker struct{}

func (m *mockTracker) SetTotalFiles(total int)                            {}
func (m *mockTracker) Start(name string, total int64) domain.ProgressTask { return nil }
func (m *mockTracker) Wait()                                              {}

func TestAccessHashCache(t *testing.T) {
	client := &TelegramClient{
		peerCache: make(map[int64]int64),
	}

	client.setAccessHash(123, 456)

	hash, ok := client.getAccessHash(123)
	if !ok || hash != 456 {
		t.Errorf("getAccessHash failed, got %d, %v", hash, ok)
	}

	_, ok = client.getAccessHash(999)
	if ok {
		t.Error("getAccessHash should return false for missing key")
	}
}

func TestParseChatsToGroups(t *testing.T) {
	client := &TelegramClient{
		peerCache: make(map[int64]int64),
	}

	chats := []tg.ChatClass{
		&tg.Channel{
			ID:         1,
			Title:      "Group 1",
			Megagroup:  true,
			AccessHash: 111,
		},
		&tg.Channel{
			ID:        2,
			Title:     "Channel 1",
			Megagroup: false, // Should be ignored
		},
		&tg.Chat{
			ID:    3,
			Title: "Chat 1", // Should be ignored
		},
	}

	groups := client.parseChatsToGroups(chats)

	if len(groups) != 1 {
		t.Errorf("Expected 1 group, got %d", len(groups))
	}
	if groups[0].ID != 1 || groups[0].Title != "Group 1" {
		t.Errorf("Group mismatch: %+v", groups[0])
	}

	// Verify access hash was cached
	hash, ok := client.getAccessHash(1)
	if !ok || hash != 111 {
		t.Errorf("AccessHash not cached correctly")
	}
}

func TestParseTopicsToDomain(t *testing.T) {
	client := &TelegramClient{}

	tgTopics := []tg.ForumTopicClass{
		&tg.ForumTopic{
			ID:    1,
			Title: "Topic 1",
		},
		&tg.ForumTopicDeleted{
			ID: 2, // Should be ignored
		},
	}

	topics := client.parseTopicsToDomain(tgTopics)

	if len(topics) != 1 {
		t.Errorf("Expected 1 topic, got %d", len(topics))
	}
	if topics[0].ID != 1 || topics[0].Title != "Topic 1" {
		t.Errorf("Topic mismatch: %+v", topics[0])
	}
}
