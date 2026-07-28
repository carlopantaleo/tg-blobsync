package usecase

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"tg-blobsync/internal/domain"
)

// TestSingleDownload_ChunkedFileStreamsChunksInOrder verifies that downloading
// a chunked file through the browse download path streams chunks in order into
// one destination file.
func TestSingleDownload_ChunkedFileStreamsChunksInOrder(t *testing.T) {
	storage := NewMockBlobStorage()
	fs := NewMockFileSystem()
	ui := &MockUserInterface{Confirmed: true}

	downloader := NewSingleDownloader(fs, storage, ui)

	req := &domain.DownloadRequest{File: domain.RemoteFile{
		Meta:      domain.FileMeta{Path: "large.bin", Flags: domain.ChunkFlag},
		Size:      30,
		MessageID: 10,
		ChunkIDs:  []int{10, 11, 12},
	}}

	if err := downloader.Download(context.Background(), req, filepath.Join("tmp", "dest"), 1, 2); err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	expectedPath := filepath.Join("tmp", "dest", "large.bin")
	data, ok := fs.Data[expectedPath]
	if !ok {
		t.Fatalf("expected file at %s", expectedPath)
	}
	// MockBlobStorage.DownloadFile returns "dummy content" per chunk
	expected := "dummy contentdummy contentdummy content"
	if string(data) != expected {
		t.Errorf("downloaded data = %q, want %q", string(data), expected)
	}
}

// TestSingleDownload_NonChunkedFileWorks verifies that non-chunked downloads
// still work through the same download path.
func TestSingleDownload_NonChunkedFileWorks(t *testing.T) {
	storage := NewMockBlobStorage()
	fs := NewMockFileSystem()
	ui := &MockUserInterface{Confirmed: true}

	downloader := NewSingleDownloader(fs, storage, ui)

	req := &domain.DownloadRequest{File: domain.RemoteFile{
		Meta:      domain.FileMeta{Path: "regular.txt"},
		Size:      100,
		MessageID: 5,
	}}

	if err := downloader.Download(context.Background(), req, filepath.Join("tmp", "dest"), 1, 2); err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	expectedPath := filepath.Join("tmp", "dest", "regular.txt")
	if _, ok := fs.Data[expectedPath]; !ok {
		t.Fatalf("expected file at %s", expectedPath)
	}
}

// TestSingleDownload_ChunkedFileRejectsIncompleteChunk verifies that a missing
// chunk causes the download to fail without writing a partial file.
func TestSingleDownload_ChunkedFileRejectsIncompleteChunk(t *testing.T) {
	storage := &failingChunkStorage{
		MockBlobStorage: *NewMockBlobStorage(),
		failOnMessageID: 11,
	}
	fs := NewMockFileSystem()
	ui := &MockUserInterface{Confirmed: true}

	downloader := NewSingleDownloader(fs, storage, ui)

	req := &domain.DownloadRequest{File: domain.RemoteFile{
		Meta:      domain.FileMeta{Path: "large.bin", Flags: domain.ChunkFlag},
		Size:      30,
		MessageID: 10,
		ChunkIDs:  []int{10, 11, 12},
	}}

	err := downloader.Download(context.Background(), req, filepath.Join("tmp", "dest"), 1, 2)
	if err == nil {
		t.Fatal("expected error for incomplete chunk download")
	}

	// The destination should not exist (no partial file)
	expectedPath := filepath.Join("tmp", "dest", "large.bin")
	if _, ok := fs.Data[expectedPath]; ok {
		t.Errorf("expected no partial file at %s", expectedPath)
	}
}

// failingChunkStorage wraps MockBlobStorage and fails DownloadFile for a
// specific message ID to simulate a missing chunk.
type failingChunkStorage struct {
	MockBlobStorage
	failOnMessageID int
}

func (m *failingChunkStorage) DownloadFile(ctx context.Context, groupID int64, topicID int64, messageID int, fileName string, size int64, task domain.ProgressTask) (io.ReadCloser, error) {
	if messageID == m.failOnMessageID {
		return nil, io.ErrUnexpectedEOF
	}
	return m.MockBlobStorage.DownloadFile(ctx, groupID, topicID, messageID, fileName, size, task)
}
