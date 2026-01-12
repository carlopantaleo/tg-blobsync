package usecase

import (
	"context"
	"testing"
	"tg-blobsync/internal/domain"
)

func TestBrowser_ListAndBrowse(t *testing.T) {
	mockStorage := NewMockBlobStorage()
	mockUI := &MockBrowseUI{}
	browser := NewBrowser(mockStorage, mockUI)

	ctx := context.Background()
	groupID := int64(100)
	topicID := int64(200)

	// 1. No files
	err := browser.ListAndBrowse(ctx, groupID, topicID)
	if err == nil {
		t.Error("Expected error when no files found, got nil")
	}

	// 2. Files found
	if mockStorage.Files[groupID] == nil {
		mockStorage.Files[groupID] = make(map[int64][]domain.RemoteFile)
	}
	mockStorage.Files[groupID][topicID] = []domain.RemoteFile{
		{Meta: domain.FileMeta{Path: "file1.txt"}},
	}

	err = browser.ListAndBrowse(ctx, groupID, topicID)
	if err != nil {
		t.Errorf("ListAndBrowse failed: %v", err)
	}

	if len(mockUI.Files) != 1 {
		t.Errorf("Expected 1 file passed to UI, got %d", len(mockUI.Files))
	}
	if mockUI.Files[0].Meta.Path != "file1.txt" {
		t.Errorf("File path mismatch")
	}
}
