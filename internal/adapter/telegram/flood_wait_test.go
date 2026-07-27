package telegram

import (
	"context"
	"errors"
	"testing"
	"tg-blobsync/internal/domain"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

func TestTelegramClient_RetriesOnFloodWait(t *testing.T) {
	mockInvoker := NewMockInvoker()

	client := &TelegramClient{
		api:            tg.NewClient(mockInvoker),
		peerCache:      make(map[int64]int64),
		progressStarts: make(map[int64]time.Time),
		progressTasks:  make(map[int64]domain.ProgressTask),
	}
	client.invoker = mockInvoker
	client.sender = message.NewSender(client.api)
	client.setAccessHash(100, 12345)

	callCount := 0
	mockInvoker.Register(&tg.MessagesGetRepliesRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		callCount++
		if callCount == 1 {
			return errors.New("rpc error code 420: FLOOD_WAIT (1)")
		}
		resp := &tg.MessagesChannelMessages{
			Messages: []tg.MessageClass{},
			Count:    0,
		}
		return encodeResponse(output, resp)
	})

	files, err := client.ListFiles(context.Background(), 100, 200)
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("Expected 0 files, got %d", len(files))
	}
	if callCount != 2 {
		t.Errorf("Expected 2 calls (1 flood wait, 1 success), got %d", callCount)
	}
}

func TestTelegramClient_FailsAfterMaxFloodWaits(t *testing.T) {
	mockInvoker := NewMockInvoker()

	client := &TelegramClient{
		api:            tg.NewClient(mockInvoker),
		peerCache:      make(map[int64]int64),
		progressStarts: make(map[int64]time.Time),
		progressTasks:  make(map[int64]domain.ProgressTask),
	}
	client.invoker = mockInvoker
	client.sender = message.NewSender(client.api)
	client.setAccessHash(100, 12345)

	callCount := 0
	mockInvoker.Register(&tg.MessagesGetRepliesRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		callCount++
		return errors.New("rpc error code 420: FLOOD_WAIT (0)") // use 0 delay for fast tests
	})

	_, err := client.ListFiles(context.Background(), 100, 200)
	if err == nil {
		t.Fatalf("Expected ListFiles to fail after max retries")
	}

	if callCount != 5 { // Default retry limit is 5
		t.Errorf("Expected 5 calls, got %d", callCount)
	}
}
