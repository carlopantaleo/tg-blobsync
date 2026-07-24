package telegram

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"tg-blobsync/internal/domain"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

func TestUploadChunkedFileSendsOrderedChunks(t *testing.T) {
	invoker := NewMockInvoker()
	invoker.Register(&tg.UploadSaveFilePartRequest{}, encodeTelegramResponse(&tg.BoolTrue{}))
	invoker.Register(&tg.UploadSaveBigFilePartRequest{}, encodeTelegramResponse(&tg.BoolTrue{}))
	nextID := 100
	invoker.Register(&tg.MessagesSendMediaRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		response := &tg.Updates{Updates: []tg.UpdateClass{&tg.UpdateNewChannelMessage{Message: &tg.Message{ID: nextID, PeerID: &tg.PeerChannel{ChannelID: 1}}}}}
		nextID++
		return encodeResponse(output, response)
	})

	path := filepath.Join(t.TempDir(), "large.bin")
	if err := os.WriteFile(path, []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	client := &TelegramClient{
		api:            tg.NewClient(invoker),
		sender:         message.NewSender(tg.NewClient(invoker)),
		uploader:       uploader.NewUploader(tg.NewClient(invoker)).WithPartSize(512 * 1024),
		peerCache:      map[int64]int64{1: 2},
		progressStarts: make(map[int64]time.Time),
		progressTasks:  make(map[int64]domain.ProgressTask),
		progressBases:  make(map[int64]int64),
		chunkThreshold: 5,
		chunkSize:      4,
	}
	client.sender = message.NewSender(client.api)
	client.uploader = uploader.NewUploader(client.api).WithPartSize(512 * 1024)

	ids, err := client.UploadChunkedFile(context.Background(), 1, 2, domain.LocalFile{Path: "large.bin", AbsPath: path, Size: 10, ModTime: 42})
	if err != nil {
		t.Fatalf("upload chunks: %v", err)
	}
	if len(ids) != 3 || ids[0] != 100 || ids[1] != 101 || ids[2] != 102 {
		t.Fatalf("chunk IDs = %#v", ids)
	}
}

func encodeTelegramResponse(response bin.Encoder) func(context.Context, bin.Encoder, bin.Decoder) error {
	return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		return encodeResponse(output, response)
	}
}

func encodeResponse(output bin.Decoder, response bin.Encoder) error {
	buffer := new(bin.Buffer)
	if err := response.Encode(buffer); err != nil {
		return err
	}
	return output.Decode(buffer)
}
