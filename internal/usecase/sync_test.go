package usecase

import (
	"context"
	"testing"
	"tg-blobsync/internal/domain"
)

func TestSynchronizer_Push(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockStorage := NewMockBlobStorage()
	mockUI := &MockUserInterface{Confirmed: true}

	// Setup FS
	mockFS.Files["file1.txt"] = domain.LocalFile{Path: "file1.txt", Size: 100}

	sync := NewSynchronizer(mockFS, mockStorage, 1, mockUI, false)

	ctx := context.Background()
	groupID := int64(1)
	topicID := int64(2)
	rootDir := "/tmp"

	err := sync.Push(ctx, rootDir, groupID, topicID)
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// Executor should have been called.
	// Since we are not mocking Executor injection (Synchronizer creates Executor internally),
	// we rely on MockStorage verifying the Upload call (or just no error returned).
	// But Executor calls storage.UploadFile.
	// We can't easily verify internal state of NewExecutor inside Synchronizer without refactoring.
	// However, we can check if it failed.
	// Also, if we set up mockStorage to fail Upload, we can verify error propagation.
}

func TestSynchronizer_PushRebuildsIndexAfterChanges(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockFS.Files["file1.txt"] = domain.LocalFile{Path: "file1.txt", Size: 100}
	mockStorage := NewMockBlobStorage()

	sync := NewSynchronizer(mockFS, mockStorage, 1, &MockUserInterface{Confirmed: true}, false)
	if err := sync.Push(context.Background(), "/tmp", 1, 2); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if mockStorage.IndexUploads != 2 {
		t.Fatalf("index uploads = %d, want migration and post-sync rebuild", mockStorage.IndexUploads)
	}
}

func TestSynchronizer_SkipsIndexRebuildWhenUpToDate(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockFS.Files["file1.txt"] = domain.LocalFile{Path: "file1.txt", Size: 100, ModTime: 10}
	mockStorage := NewMockBlobStorage()
	mockStorage.Indexes[1] = map[int64]*domain.FileIndex{2: {Entries: []domain.FileIndexEntry{{Path: "file1.txt", Size: 100, ModTime: 10}}}}
	mockStorage.IndexIDs[1] = map[int64]int{2: 10}

	sync := NewSynchronizer(mockFS, mockStorage, 1, &MockUserInterface{Confirmed: true}, true)
	if err := sync.Push(context.Background(), "/tmp", 1, 2); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if mockStorage.IndexUploads != 0 {
		t.Fatalf("index uploads = %d, want 0 for no-op sync", mockStorage.IndexUploads)
	}
}

func TestSynchronizer_Pull(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockStorage := NewMockBlobStorage()
	mockUI := &MockUserInterface{Confirmed: true}

	// Setup Remote
	mockStorage.Files[1] = map[int64][]domain.RemoteFile{
		2: {{Meta: domain.FileMeta{Path: "remote.txt"}, Size: 100, MessageID: 1}},
	}

	sync := NewSynchronizer(mockFS, mockStorage, 1, mockUI, false)

	ctx := context.Background()
	groupID := int64(1)
	topicID := int64(2)
	rootDir := "/tmp"

	err := sync.Pull(ctx, rootDir, groupID, topicID)
	if err != nil {
		t.Fatalf("Pull failed: %v", err)
	}

	// Check if file was written (downloaded)
	// Executor logic writes to fs.
	// Note: FS path depends on Executor implementation.
	// Ideally we should check mockFS.Data
}

func TestSynchronizer_SetSubDir(t *testing.T) {
	s := NewSynchronizer(NewMockFileSystem(), NewMockBlobStorage(), 1, &MockUserInterface{}, false)
	s.SetSubDir("nested")

	if s.subDir != "nested" {
		t.Fatalf("subDir = %s, want nested", s.subDir)
	}
}
