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

	// 2. Scan with Subdir
	// ScanLocal should return all files relative to rootDir, regardless of subDir
	// The subDir logic is used to filter remote files and map them to the same relative root.
	scannerSub := NewScanner(mockFS, mockStorage, "remote-sub", false)
	filesSub, err := scannerSub.ScanLocal("/tmp")
	if err != nil {
		t.Fatalf("ScanLocal failed: %v", err)
	}
	if len(filesSub) != 2 {
		t.Errorf("Expected 2 files, got %d", len(filesSub))
	}
	if _, ok := filesSub["file1.txt"]; !ok {
		t.Error("file1.txt missing")
	}
}

func TestScanner_ScanRemoteUsesIndex(t *testing.T) {
	mockStorage := NewMockBlobStorage()
	groupID := int64(1)
	topicID := int64(2)
	mockStorage.Indexes[groupID] = map[int64]*domain.FileIndex{
		topicID: {Entries: []domain.FileIndexEntry{{Path: "remote-sub/file.txt", Size: 12, MessageID: 7}}},
	}
	mockStorage.IndexIDs[groupID] = map[int64]int{topicID: 9}

	scanner := NewScanner(NewMockFileSystem(), mockStorage, "remote-sub", false)
	files, err := scanner.ScanRemote(context.Background(), groupID, topicID)
	if err != nil {
		t.Fatalf("ScanRemote failed: %v", err)
	}
	file, ok := files["file.txt"]
	if !ok || file.MessageID != 7 || file.Size != 12 {
		t.Fatalf("indexed file = %#v, want message 7 and size 12", file)
	}
}

func TestScanner_ScanRemoteMigratesLegacyTopic(t *testing.T) {
	mockStorage := NewMockBlobStorage()
	groupID := int64(1)
	topicID := int64(2)
	mockStorage.Files[groupID] = map[int64][]domain.RemoteFile{
		topicID: {{Meta: domain.FileMeta{Path: "file.txt"}, MessageID: 3, Size: 12}},
	}

	scanner := NewScanner(NewMockFileSystem(), mockStorage, "", false)
	files, err := scanner.ScanRemote(context.Background(), groupID, topicID)
	if err != nil {
		t.Fatalf("ScanRemote failed: %v", err)
	}
	if len(files) != 1 || mockStorage.Indexes[groupID][topicID] == nil {
		t.Fatalf("legacy scan did not migrate index: files=%#v indexes=%#v", files, mockStorage.Indexes)
	}

	files, err = scanner.ScanRemote(context.Background(), groupID, topicID)
	if err != nil {
		t.Fatalf("indexed ScanRemote failed: %v", err)
	}
	if len(files) != 1 || files["file.txt"].MessageID != 3 {
		t.Fatalf("indexed scan returned %#v, want migrated file", files)
	}
}

func TestScanner_ScanRemoteRemovesStaleIndexes(t *testing.T) {
	mockStorage := NewMockBlobStorage()
	mockStorage.Files[1] = map[int64][]domain.RemoteFile{2: {{Meta: domain.FileMeta{Path: "file.txt"}, MessageID: 3}}}
	mockStorage.StaleIndexIDs[1] = map[int64][]int{2: {10, 11}}

	if _, err := NewScanner(NewMockFileSystem(), mockStorage, "", false).ScanRemote(context.Background(), 1, 2); err != nil {
		t.Fatalf("ScanRemote failed: %v", err)
	}
	if len(mockStorage.DeletedIDs) != 2 || mockStorage.IndexUploads != 1 {
		t.Fatalf("deleted IDs=%v uploads=%d, want stale cleanup and migration", mockStorage.DeletedIDs, mockStorage.IndexUploads)
	}
}

func TestScanner_ScanRemote(t *testing.T) {
	mockStorage := NewMockBlobStorage()
	groupID := int64(1)
	topicID := int64(2)

	mockStorage.Files[groupID] = make(map[int64][]domain.RemoteFile)
	mockStorage.Files[groupID][topicID] = []domain.RemoteFile{
		{Meta: domain.FileMeta{Path: "file1.txt"}},
		{Meta: domain.FileMeta{Path: "remote-sub/file2.txt"}},
		{Meta: domain.FileMeta{Path: "remote-sub/nested/file3.txt"}},
	}

	mockFS := NewMockFileSystem()

	// 1. Scan All
	scanner := NewScanner(mockFS, mockStorage, "", false)
	files, err := scanner.ScanRemote(context.Background(), groupID, topicID)
	if err != nil {
		t.Fatalf("ScanRemote failed: %v", err)
	}
	if len(files) != 3 {
		t.Errorf("Expected 3 files, got %d", len(files))
	}

	// 2. Scan Subdir
	// When subDir is "remote-sub", "remote-sub/file2.txt" should become "file2.txt"
	scannerSub := NewScanner(mockFS, mockStorage, "remote-sub", false)
	filesSub, err := scannerSub.ScanRemote(context.Background(), groupID, topicID)
	if err != nil {
		t.Fatalf("ScanRemote failed: %v", err)
	}
	if len(filesSub) != 2 {
		t.Errorf("Expected 2 files in subdir, got %d", len(filesSub))
	}
	if _, ok := filesSub["file2.txt"]; !ok {
		t.Error("file2.txt missing")
	}
	if _, ok := filesSub["nested/file3.txt"]; !ok {
		t.Error("nested/file3.txt missing")
	}
}
