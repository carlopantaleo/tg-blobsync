package usecase

import (
	"context"
	"testing"
	"tg-blobsync/internal/domain"
)

func TestScanner_ScanLocal(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockFS.Files["file1.txt"] = domain.LocalFile{Path: "file1.txt", Size: 100}
	mockFS.Files["subdir/file2.txt"] = domain.LocalFile{Path: "subdir/file2.txt", Size: 200}

	mockStorage := NewMockBlobStorage()

	// 1. Scan All
	scanner := NewScanner(mockFS, mockStorage, "", false)
	files, err := scanner.ScanLocal("/tmp")
	if err != nil {
		t.Fatalf("ScanLocal failed: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}
	if _, ok := files["file1.txt"]; !ok {
		t.Error("file1.txt missing")
	}

	// 2. Scan Subdir
	scannerSub := NewScanner(mockFS, mockStorage, "subdir", false)
	filesSub, err := scannerSub.ScanLocal("/tmp")
	if err != nil {
		t.Fatalf("ScanLocal failed: %v", err)
	}
	if len(filesSub) != 1 {
		t.Errorf("Expected 1 file in subdir, got %d", len(filesSub))
	}
	if _, ok := filesSub["subdir/file2.txt"]; !ok {
		t.Error("subdir/file2.txt missing")
	}
}

func TestScanner_ScanRemote(t *testing.T) {
	mockStorage := NewMockBlobStorage()
	groupID := int64(1)
	topicID := int64(2)

	mockStorage.Files[groupID] = make(map[int64][]domain.RemoteFile)
	mockStorage.Files[groupID][topicID] = []domain.RemoteFile{
		{Meta: domain.FileMeta{Path: "file1.txt"}},
		{Meta: domain.FileMeta{Path: "subdir/file2.txt"}},
		// Duplicate file (older version usually, but here just checking dedup logic if implemented or map behavior)
		{Meta: domain.FileMeta{Path: "file1.txt"}},
	}

	mockFS := NewMockFileSystem()

	// 1. Scan All
	scanner := NewScanner(mockFS, mockStorage, "", false)
	files, err := scanner.ScanRemote(context.Background(), groupID, topicID)
	if err != nil {
		t.Fatalf("ScanRemote failed: %v", err)
	}

	// Should dedup based on map key
	if len(files) != 2 {
		t.Errorf("Expected 2 unique files, got %d", len(files))
	}

	// 2. Scan Subdir
	scannerSub := NewScanner(mockFS, mockStorage, "subdir", false)
	filesSub, err := scannerSub.ScanRemote(context.Background(), groupID, topicID)
	if err != nil {
		t.Fatalf("ScanRemote failed: %v", err)
	}
	if len(filesSub) != 1 {
		t.Errorf("Expected 1 file in subdir, got %d", len(filesSub))
	}
	if _, ok := filesSub["subdir/file2.txt"]; !ok {
		t.Error("subdir/file2.txt missing")
	}
}
