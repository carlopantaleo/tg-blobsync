package telegram

import (
	"context"
	"errors"
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

func TestTelegramClient_UploadFile(t *testing.T) {
	mockInvoker := NewMockInvoker()

	client := &TelegramClient{
		api:            tg.NewClient(mockInvoker),
		peerCache:      make(map[int64]int64),
		progressStarts: make(map[int64]time.Time),
		progressTasks:  make(map[int64]domain.ProgressTask),
	}
	client.sender = message.NewSender(client.api)
	client.setAccessHash(100, 12345)

	// Initialize uploader manually as Start() is not called
	client.uploader = uploader.NewUploader(client.api).
		WithPartSize(512 * 1024).
		WithThreads(1)

	// Create a temporary file to upload
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "upload_test.txt")
	content := []byte("hello world")
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	localFile := domain.LocalFile{
		Path:    "upload_test.txt",
		AbsPath: tmpFile,
		Size:    int64(len(content)),
		ModTime: time.Now().Unix(),
	}

	// Mock upload.saveBigFilePart (or saveFilePart depending on size/uploader logic)
	// For small files, gotd uploader might use saveFilePart.
	// Since size is small ("hello world"), it should be saveFilePart.
	mockInvoker.Register(&tg.UploadSaveFilePartRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		resp := &tg.BoolTrue{} // upload.saveFilePart returns Bool
		buf := new(bin.Buffer)
		if err := resp.Encode(buf); err != nil {
			return err
		}
		return output.Decode(buf)
	})

	// Mock messages.sendMedia
	mockInvoker.Register(&tg.MessagesSendMediaRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		req := input.(*tg.MessagesSendMediaRequest)
		// Verify Request
		peer, ok := req.Peer.(*tg.InputPeerChannel)
		if !ok || peer.ChannelID != 100 {
			return errors.New("invalid peer")
		}

		// Response: Updates (usually)
		resp := &tg.Updates{}
		buf := new(bin.Buffer)
		if err := resp.Encode(buf); err != nil {
			return err
		}
		return output.Decode(buf)
	})

	err := client.UploadFile(context.Background(), 100, 200, localFile)
	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}
}

func TestTelegramClient_UploadFile_Empty(t *testing.T) {
	mockInvoker := NewMockInvoker()

	client := &TelegramClient{
		api:            tg.NewClient(mockInvoker),
		peerCache:      make(map[int64]int64),
		progressStarts: make(map[int64]time.Time),
		progressTasks:  make(map[int64]domain.ProgressTask),
	}
	client.sender = message.NewSender(client.api)
	client.setAccessHash(100, 12345)

	client.uploader = uploader.NewUploader(client.api).WithThreads(1)

	localFile := domain.LocalFile{
		Path: "empty.txt",
		Size: 0,
	}

	// Expect saveFilePart for the 1-byte dummy
	mockInvoker.Register(&tg.UploadSaveFilePartRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		resp := &tg.BoolTrue{}
		buf := new(bin.Buffer)
		if err := resp.Encode(buf); err != nil {
			return err
		}
		return output.Decode(buf)
	})

	// Expect messages.sendMedia
	mockInvoker.Register(&tg.MessagesSendMediaRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		resp := &tg.Updates{}
		buf := new(bin.Buffer)
		if err := resp.Encode(buf); err != nil {
			return err
		}
		return output.Decode(buf)
	})

	err := client.UploadFile(context.Background(), 100, 200, localFile)
	if err != nil {
		t.Fatalf("UploadFile(Empty) failed: %v", err)
	}
}

func TestTelegramClient_DownloadFile(t *testing.T) {
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
					Size:          10, // content length
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

	// 2. Mock upload.getFile to return content
	// gotd downloader uses upload.getFile
	mockInvoker.Register(&tg.UploadGetFileRequest{}, func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		resp := &tg.UploadFile{
			Type:  &tg.StorageFilePartial{},
			Mtime: int(time.Now().Unix()),
			Bytes: []byte("helloworld"), // 10 bytes
		}
		buf := new(bin.Buffer)
		if err := resp.Encode(buf); err != nil {
			return err
		}
		return output.Decode(buf)
	})

	rc, err := client.DownloadFile(context.Background(), 100, 200, 123, "test.txt", 10)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	// Read content
	// Since DownloadFile spawns a goroutine to write to the pipe, we read from rc
	data := make([]byte, 20)
	n, err := rc.Read(data)
	// EOF is expected after full read, but Read might return n > 0 and err == nil first
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("Read failed: %v", err)
	}

	if n != 10 {
		t.Errorf("Expected 10 bytes, got %d", n)
	}
	if string(data[:n]) != "helloworld" {
		t.Errorf("Content mismatch: %s", string(data[:n]))
	}

	rc.Close()
}
