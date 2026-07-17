package telegram

import (
	"context"
	"testing"
	"time"

	"tg-blobsync/internal/domain"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

func TestGetIndexReturnsAbsentWhenLastMessageIsNotIndex(t *testing.T) {
	client, invoker := newIndexTestClient()
	invoker.Register(&tg.MessagesGetRepliesRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		response := &tg.MessagesChannelMessages{
			Messages: []tg.MessageClass{&tg.Message{
				ID:      9,
				PeerID:  &tg.PeerChannel{ChannelID: 100},
				Message: `{"p":"file.txt","m":"checksum"}`,
				Date:    int(time.Now().Unix()),
			}},
			Count: 1,
		}
		buffer := new(bin.Buffer)
		if err := response.Encode(buffer); err != nil {
			return err
		}
		return output.Decode(buffer)
	})

	index, messageID, present, err := client.GetIndex(context.Background(), 100, 200)
	if err != nil {
		t.Fatalf("GetIndex failed: %v", err)
	}
	if present || index != nil || messageID != 0 {
		t.Fatalf("GetIndex = (%#v, %d, %t), want absent", index, messageID, present)
	}
}

func TestGetIndexReturnsAbsentForEmptyTopic(t *testing.T) {
	client, invoker := newIndexTestClient()
	invoker.Register(&tg.MessagesGetRepliesRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		response := &tg.MessagesChannelMessages{Messages: []tg.MessageClass{}, Count: 0}
		buffer := new(bin.Buffer)
		if err := response.Encode(buffer); err != nil {
			return err
		}
		return output.Decode(buffer)
	})

	_, _, present, err := client.GetIndex(context.Background(), 100, 200)
	if err != nil {
		t.Fatalf("GetIndex failed: %v", err)
	}
	if present {
		t.Fatal("GetIndex reported an index for an empty topic")
	}
}

func TestIsIndexMessage(t *testing.T) {
	if !isIndexMessage(&tg.Message{Message: `{"f":"INDEX"}`}) {
		t.Fatal("expected INDEX marker to be recognized")
	}
	if isIndexMessage(&tg.Message{Message: `{"f":"EMPTY_FILE"}`}) {
		t.Fatal("did not expect EMPTY_FILE marker to be recognized as INDEX")
	}
}

func TestMessageIDFromUpdates(t *testing.T) {
	updates := &tg.Updates{Updates: []tg.UpdateClass{
		&tg.UpdateNewChannelMessage{Message: &tg.Message{ID: 77}},
	}}
	messageID, ok := messageIDFromUpdates(updates)
	if !ok || messageID != 77 {
		t.Fatalf("messageIDFromUpdates = (%d, %t), want (77, true)", messageID, ok)
	}
}

func newIndexTestClient() (*TelegramClient, *MockInvoker) {
	invoker := NewMockInvoker()
	client := &TelegramClient{
		api:            tg.NewClient(invoker),
		peerCache:      map[int64]int64{100: 12345},
		progressStarts: make(map[int64]time.Time),
		progressTasks:  make(map[int64]domain.ProgressTask),
	}
	client.sender = message.NewSender(client.api)
	return client, invoker
}
