package telegram

import (
	"context"
	"testing"
	"time"

	"tg-blobsync/internal/domain"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

type mockExternalTask struct {
	completed bool
	aborted   bool
}

func (m *mockExternalTask) Increment(n int)    {}
func (m *mockExternalTask) SetCurrent(n int64) {}
func (m *mockExternalTask) SetChunk(c, t int)  {}
func (m *mockExternalTask) Complete()          { m.completed = true }
func (m *mockExternalTask) Abort()             { m.aborted = true }

func TestTelegramClient_DownloadFile_DoesNotCompleteExternalTask(t *testing.T) {
	mockInvoker := NewMockInvoker()

	client := &TelegramClient{
		api:            tg.NewClient(mockInvoker),
		peerCache:      make(map[int64]int64),
		progressStarts: make(map[int64]time.Time),
		progressTasks:  make(map[int64]domain.ProgressTask),
	}
	client.setAccessHash(100, 12345)

	mockInvoker.Register(&tg.ChannelsGetMessagesRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		msg := &tg.Message{
			ID:     123,
			Date:   int(time.Now().Unix()),
			PeerID: &tg.PeerChannel{ChannelID: 100},
			Media: &tg.MessageMediaDocument{
				Document: &tg.Document{
					ID:            11,
					AccessHash:    22,
					FileReference: []byte{1},
					Date:          int(time.Now().Unix()),
					Size:          10,
					DCID:          1,
					MimeType:      "text/plain",
				},
			},
		}
		resp := &tg.MessagesChannelMessages{
			Messages: []tg.MessageClass{msg},
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

	mockInvoker.Register(&tg.UploadGetFileRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		resp := &tg.UploadFile{
			Type:  &tg.StorageFilePartial{},
			Mtime: int(time.Now().Unix()),
			Bytes: []byte("helloworld"),
		}
		buf := new(bin.Buffer)
		if err := resp.Encode(buf); err != nil {
			return err
		}
		return output.Decode(buf)
	})

	task := &mockExternalTask{}
	rc, err := client.DownloadFile(context.Background(), 100, 200, 123, "test.txt", 10, task)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	// Read content
	data := make([]byte, 20)
	rc.Read(data)
	rc.Close()

	// Give the goroutine a tiny bit to finish and potentially call Complete
	time.Sleep(50 * time.Millisecond)

	if task.completed {
		t.Error("DownloadFile should not complete external task")
	}
	if task.aborted {
		t.Error("DownloadFile should not abort external task")
	}
}
