package usecase

import (
	"context"
	"errors"
	"io"
	"testing"
	"tg-blobsync/internal/domain"
)

// MockFSError allows simulating FS errors
type MockFSError struct {
	MockFileSystem
	ReadErr  error
	WriteErr error
	ListErr  error
}

func (m *MockFSError) ReadFile(path string) (io.ReadCloser, error) {
	if m.ReadErr != nil {
		return nil, m.ReadErr
	}
	return m.MockFileSystem.ReadFile(path)
}

// We need to implement the interface correctly.
func (m *MockFSError) ListFiles(root string, skipMD5 bool) ([]domain.LocalFile, error) {
	if m.ListErr != nil {
		return nil, m.ListErr
	}
	return m.MockFileSystem.ListFiles(root, skipMD5)
}

// MockStorageError allows simulating Storage errors
type MockStorageError struct {
	MockBlobStorage
	ListErr   error
	UploadErr error
}

func (m *MockStorageError) ListFiles(ctx context.Context, groupID int64, topicID int64) ([]domain.RemoteFile, error) {
	if m.ListErr != nil {
		return nil, m.ListErr
	}
	return m.MockBlobStorage.ListFiles(ctx, groupID, topicID)
}

func (m *MockStorageError) UploadFile(ctx context.Context, groupID int64, topicID int64, file domain.LocalFile) error {
	if m.UploadErr != nil {
		return m.UploadErr
	}
	return m.MockBlobStorage.UploadFile(ctx, groupID, topicID, file)
}

func TestScanner_Errors(t *testing.T) {
	// 1. ScanLocal Error
	mockFS := &MockFSError{
		MockFileSystem: *NewMockFileSystem(),
		ListErr:        errors.New("fs error"),
	}
	mockStorage := NewMockBlobStorage()

	scanner := NewScanner(mockFS, mockStorage, "", false)
	_, err := scanner.ScanLocal("/tmp")
	if err == nil {
		t.Error("Expected error from ScanLocal")
	}

	// 2. ScanRemote Error
	mockStorageErr := &MockStorageError{
		MockBlobStorage: *NewMockBlobStorage(),
		ListErr:         errors.New("net error"),
	}
	scanner2 := NewScanner(NewMockFileSystem(), mockStorageErr, "", false)
	_, err = scanner2.ScanRemote(context.Background(), 1, 2)
	if err == nil {
		t.Error("Expected error from ScanRemote")
	}
}

func TestExecutor_UploadError(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockStorage := &MockStorageError{
		MockBlobStorage: *NewMockBlobStorage(),
		UploadErr:       errors.New("upload fail"),
	}
	mockUI := &MockUserInterface{Confirmed: true}

	executor := NewExecutor(mockFS, mockStorage, 1, mockUI)

	localFile := domain.LocalFile{Path: "test.txt", AbsPath: "test.txt"}
	plan := domain.SyncPlan{
		Items: []domain.SyncItem{
			{
				Path:      "test.txt",
				Action:    domain.ActionUpload,
				LocalFile: &localFile,
			},
		},
		Summary: domain.SyncSummary{Total: 1, ToUpload: 1},
	}

	err := executor.Execute(context.Background(), plan, "/tmp", 1, 2)
	// Executor currently uses errgroup, so it should return the first error
	if err == nil {
		t.Error("Expected error from Execute due to upload failure")
	}
}
