package usecase

import (
	"context"
	"path/filepath"
	"testing"

	"tg-blobsync/internal/domain"
)

func containsID(ids []int, target int) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func TestSynchronizer_PushNoDelete(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockFS.Files["new.txt"] = domain.LocalFile{Path: "new.txt", ModTime: 100, Size: 10}

	mockStorage := NewMockBlobStorage()
	mockStorage.Files[100] = map[int64][]domain.RemoteFile{
		200: {{Meta: domain.FileMeta{Path: "old.txt", ModTime: 400}, Size: 40, MessageID: 555}},
	}
	mockUI := &MockUserInterface{Confirmed: true}

	syncer := NewSynchronizer(mockFS, mockStorage, 1, mockUI, false)
	syncer.SetNoDelete(true)

	if err := syncer.Push(context.Background(), filepath.Join("tmp", "test"), 100, 200); err != nil {
		t.Fatalf("Push() failed: %v", err)
	}

	if containsID(mockStorage.DeletedIDs, 555) {
		t.Error("remote-only file 555 was deleted despite noDelete")
	}
	if mockStorage.UploadFileCalls != 1 {
		t.Errorf("expected 1 upload, got %d", mockStorage.UploadFileCalls)
	}
}

func TestSynchronizer_PushDeletesByDefault(t *testing.T) {
	mockFS := NewMockFileSystem()

	mockStorage := NewMockBlobStorage()
	mockStorage.Files[100] = map[int64][]domain.RemoteFile{
		200: {{Meta: domain.FileMeta{Path: "old.txt", ModTime: 400}, Size: 40, MessageID: 555}},
	}
	mockUI := &MockUserInterface{Confirmed: true}

	syncer := NewSynchronizer(mockFS, mockStorage, 1, mockUI, false)

	if err := syncer.Push(context.Background(), filepath.Join("tmp", "test"), 100, 200); err != nil {
		t.Fatalf("Push() failed: %v", err)
	}

	if !containsID(mockStorage.DeletedIDs, 555) {
		t.Error("remote-only file 555 was not deleted without noDelete")
	}
}

func TestSynchronizer_PullNoDelete(t *testing.T) {
	rootDir := filepath.Join("tmp", "test")
	mockFS := NewMockFileSystem()
	mockFS.Files["local-only.txt"] = domain.LocalFile{Path: "local-only.txt", ModTime: 100, Size: 10}
	localPath := filepath.Join(rootDir, "local-only.txt")
	mockFS.Data[localPath] = []byte("content")

	mockStorage := NewMockBlobStorage()
	mockUI := &MockUserInterface{Confirmed: true}

	syncer := NewSynchronizer(mockFS, mockStorage, 1, mockUI, false)
	syncer.SetNoDelete(true)

	if err := syncer.Pull(context.Background(), rootDir, 100, 200); err != nil {
		t.Fatalf("Pull() failed: %v", err)
	}

	if _, ok := mockFS.Data[localPath]; !ok {
		t.Error("local-only file was deleted despite noDelete")
	}
}

func TestSynchronizer_PullDeletesByDefault(t *testing.T) {
	rootDir := filepath.Join("tmp", "test")
	mockFS := NewMockFileSystem()
	mockFS.Files["local-only.txt"] = domain.LocalFile{Path: "local-only.txt", ModTime: 100, Size: 10}
	localPath := filepath.Join(rootDir, "local-only.txt")
	mockFS.Data[localPath] = []byte("content")

	mockStorage := NewMockBlobStorage()
	mockUI := &MockUserInterface{Confirmed: true}

	syncer := NewSynchronizer(mockFS, mockStorage, 1, mockUI, false)

	if err := syncer.Pull(context.Background(), rootDir, 100, 200); err != nil {
		t.Fatalf("Pull() failed: %v", err)
	}

	if _, ok := mockFS.Data[localPath]; ok {
		t.Error("local-only file was not deleted without noDelete")
	}
}
