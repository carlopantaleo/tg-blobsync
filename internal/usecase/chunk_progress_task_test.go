package usecase

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"tg-blobsync/internal/domain"
)

// taskAwareStorage is a MockBlobStorage that records the ProgressTask passed to
// DownloadFile/DownloadChunkedFile and reports chunk transitions back to it.
type taskAwareStorage struct {
	MockBlobStorage
	mu                sync.Mutex
	downloadFileCalls int
	chunkedFileCalls  int
	receivedTask      domain.ProgressTask
	chunkUpdates      [][2]int
}

func (m *taskAwareStorage) DownloadFile(ctx context.Context, groupID int64, topicID int64, messageID int, fileName string, size int64, task domain.ProgressTask) (io.ReadCloser, error) {
	m.mu.Lock()
	m.downloadFileCalls++
	m.mu.Unlock()
	return io.NopCloser(strings.NewReader("dummy content")), nil
}

func (m *taskAwareStorage) DownloadChunkedFile(ctx context.Context, groupID, topicID int64, chunkIDs []int, fileName string, size int64, task domain.ProgressTask) (io.ReadCloser, error) {
	m.mu.Lock()
	m.chunkedFileCalls++
	m.receivedTask = task
	m.mu.Unlock()
	// Simulate per-chunk progress updates as the reader is consumed
	return newChunkProgressReader(chunkIDs, task), nil
}

// chunkProgressReader is a reader that calls SetChunk on the task for each
// chunk it yields, simulating dynamic per-chunk progress reporting.
type chunkProgressReader struct {
	ids     []int
	idx     int
	task    domain.ProgressTask
	current io.Reader
}

func newChunkProgressReader(ids []int, task domain.ProgressTask) *chunkProgressReader {
	r := &chunkProgressReader{ids: ids, task: task}
	r.advance()
	return r
}

func (r *chunkProgressReader) advance() {
	if r.idx >= len(r.ids) {
		r.current = nil
		return
	}
	if r.task != nil {
		r.task.SetChunk(r.idx+1, len(r.ids))
	}
	r.current = strings.NewReader("dummy content")
	r.idx++
}

func (r *chunkProgressReader) Read(p []byte) (int, error) {
	for {
		if r.current == nil {
			return 0, io.EOF
		}
		n, err := r.current.Read(p)
		if err == io.EOF {
			r.advance()
			if r.current == nil {
				return n, io.EOF
			}
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func (r *chunkProgressReader) Close() error { return nil }

// TestExecutor_ChunkedDownloadPassesTaskToStorage verifies that the executor
// passes the logical progress task down to the storage layer's
// DownloadChunkedFile, so the storage can update chunk progress dynamically
// instead of creating new tasks per chunk.
func TestExecutor_ChunkedDownloadPassesTaskToStorage(t *testing.T) {
	storage := &taskAwareStorage{MockBlobStorage: *NewMockBlobStorage()}
	fs := NewMockFileSystem()
	ui := newRecordingUI()
	executor := NewExecutor(fs, storage, 1, ui)

	remote := domain.RemoteFile{
		Meta:     domain.FileMeta{Path: "large.bin"},
		ChunkIDs: []int{10, 11, 12},
		Size:     30,
	}
	plan := domain.SyncPlan{
		Items: []domain.SyncItem{{
			Path:       "large.bin",
			Action:     domain.ActionDownload,
			RemoteFile: &remote,
		}},
		Summary: domain.SyncSummary{Total: 1, ToDownload: 1},
	}

	if _, err := executor.Execute(context.Background(), plan, "root", 1, 2); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if storage.chunkedFileCalls != 1 {
		t.Errorf("DownloadChunkedFile calls = %d, want 1", storage.chunkedFileCalls)
	}
	if storage.receivedTask == nil {
		t.Fatal("expected storage to receive the logical progress task, got nil")
	}
}

// TestSingleDownloader_ChunkedDownloadPassesTaskToStorage verifies that the
// SingleDownloader passes the logical progress task down to the storage
// layer's DownloadChunkedFile for browse downloads.
func TestSingleDownloader_ChunkedDownloadPassesTaskToStorage(t *testing.T) {
	storage := &taskAwareStorage{MockBlobStorage: *NewMockBlobStorage()}
	fs := NewMockFileSystem()
	ui := newRecordingUI()
	downloader := NewSingleDownloader(fs, storage, ui)

	req := &domain.DownloadRequest{File: domain.RemoteFile{
		Meta:     domain.FileMeta{Path: "large.bin", Flags: domain.ChunkFlag},
		Size:     30,
		ChunkIDs: []int{10, 11, 12},
	}}

	if err := downloader.Download(context.Background(), req, filepath.Join("tmp", "dest"), 1, 2); err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if storage.chunkedFileCalls != 1 {
		t.Errorf("DownloadChunkedFile calls = %d, want 1", storage.chunkedFileCalls)
	}
	if storage.receivedTask == nil {
		t.Fatal("expected storage to receive the logical progress task, got nil")
	}
}

// TestExecutor_ChunkedDownloadUpdatesChunkIndexDynamically verifies that the
// storage layer updates the active chunk index on the logical task as each
// chunk is streamed, producing per-chunk progress calls (1/3, 2/3, 3/3).
func TestExecutor_ChunkedDownloadUpdatesChunkIndexDynamically(t *testing.T) {
	storage := &taskAwareStorage{MockBlobStorage: *NewMockBlobStorage()}
	fs := NewMockFileSystem()
	ui := newRecordingUI()
	executor := NewExecutor(fs, storage, 1, ui)

	remote := domain.RemoteFile{
		Meta:     domain.FileMeta{Path: "large.bin"},
		ChunkIDs: []int{10, 11, 12},
		Size:     30,
	}
	plan := domain.SyncPlan{
		Items: []domain.SyncItem{{
			Path:       "large.bin",
			Action:     domain.ActionDownload,
			RemoteFile: &remote,
		}},
		Summary: domain.SyncSummary{Total: 1, ToDownload: 1},
	}

	if _, err := executor.Execute(context.Background(), plan, "root", 1, 2); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(ui.tasks) != 1 {
		t.Fatalf("expected 1 progress task, got %d", len(ui.tasks))
	}
	task := ui.tasks[0]
	// The storage layer should have called SetChunk for each chunk (1/3, 2/3, 3/3)
	if len(task.chunks) < 3 {
		t.Errorf("expected at least 3 SetChunk calls from storage, got %d", len(task.chunks))
	}
	// Verify the first chunk call reports 1/3
	if len(task.chunks) > 0 {
		first := task.chunks[0]
		if first[0] != 1 || first[1] != 3 {
			t.Errorf("first SetChunk = %d/%d, want 1/3", first[0], first[1])
		}
	}
	// Verify the last chunk call reports 3/3
	if len(task.chunks) > 0 {
		last := task.chunks[len(task.chunks)-1]
		if last[0] != 3 || last[1] != 3 {
			t.Errorf("last SetChunk = %d/%d, want 3/3", last[0], last[1])
		}
	}
}
