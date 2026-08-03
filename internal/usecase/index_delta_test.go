package usecase

import (
	"context"
	"testing"
	"tg-blobsync/internal/domain"
)

// TestSynchronizer_IndexedPushDoesNotListFilesDuringRebuild verifies that when a
// synchronization starts from a valid index, the post-sync index update does
// NOT call ListFiles or ListIndexMessageIDs (full-topic scans).
func TestSynchronizer_IndexedPushDoesNotListFilesDuringRebuild(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockFS.Files["new.txt"] = domain.LocalFile{Path: "new.txt", Size: 50}

	mockStorage := NewMockBlobStorage()
	// Pre-populate an existing index
	mockStorage.Indexes[1] = map[int64]*domain.FileIndex{2: {Entries: []domain.FileIndexEntry{
		{Path: "old.txt", Size: 100, MessageID: 100},
	}}}
	mockStorage.IndexIDs[1] = map[int64]int{2: 500}

	sync := NewSynchronizer(mockFS, mockStorage, 1, &MockUserInterface{Confirmed: true}, false)
	if err := sync.Push(context.Background(), "/tmp", 1, 2); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	if mockStorage.ListFilesCalls > 0 {
		t.Errorf("ListFiles should not be called during indexed sync, got %d calls", mockStorage.ListFilesCalls)
	}
	if mockStorage.ListIndexMessageIDsCalls > 0 {
		t.Errorf("ListIndexMessageIDs should not be called during indexed sync, got %d calls", mockStorage.ListIndexMessageIDsCalls)
	}
	if mockStorage.IndexUploads != 1 {
		t.Errorf("expected 1 index upload (delta update), got %d", mockStorage.IndexUploads)
	}
}

// TestSynchronizer_IndexedPushAppliesUploadDelta verifies that the post-sync
// index reflects the uploaded file added to the retained snapshot.
func TestSynchronizer_IndexedPushAppliesUploadDelta(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockFS.Files["old.txt"] = domain.LocalFile{Path: "old.txt", Size: 100, ModTime: 10}
	mockFS.Files["new.txt"] = domain.LocalFile{Path: "new.txt", Size: 50, ModTime: 20}

	mockStorage := NewMockBlobStorage()
	mockStorage.Indexes[1] = map[int64]*domain.FileIndex{2: {Entries: []domain.FileIndexEntry{
		{Path: "old.txt", Size: 100, MessageID: 100, ModTime: 10},
	}}}
	mockStorage.IndexIDs[1] = map[int64]int{2: 500}

	sync := NewSynchronizer(mockFS, mockStorage, 1, &MockUserInterface{Confirmed: true}, true)
	if err := sync.Push(context.Background(), "/tmp", 1, 2); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	index := mockStorage.Indexes[1][2]
	if index == nil {
		t.Fatal("expected updated index")
	}
	paths := make(map[string]bool)
	for _, e := range index.Entries {
		paths[e.Path] = true
	}
	if !paths["old.txt"] {
		t.Errorf("expected old.txt to be retained in delta-updated index")
	}
	if !paths["new.txt"] {
		t.Errorf("expected new.txt to be added in delta-updated index")
	}
}

// TestSynchronizer_IndexedPushAppliesDeleteDelta verifies that deleted remote
// files are removed from the retained index snapshot.
func TestSynchronizer_IndexedPushAppliesDeleteDelta(t *testing.T) {
	mockFS := NewMockFileSystem()
	// No local files - so remote "old.txt" should be deleted

	mockStorage := NewMockBlobStorage()
	mockStorage.Indexes[1] = map[int64]*domain.FileIndex{2: {Entries: []domain.FileIndexEntry{
		{Path: "old.txt", Size: 100, MessageID: 100},
	}}}
	mockStorage.IndexIDs[1] = map[int64]int{2: 500}

	sync := NewSynchronizer(mockFS, mockStorage, 1, &MockUserInterface{Confirmed: true}, false)
	if err := sync.Push(context.Background(), "/tmp", 1, 2); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	index := mockStorage.Indexes[1][2]
	if index == nil {
		t.Fatal("expected updated index")
	}
	for _, e := range index.Entries {
		if e.Path == "old.txt" {
			t.Errorf("expected old.txt to be removed from delta-updated index")
		}
	}
}

// TestSynchronizer_IndexedPushAppliesUpdateDelta verifies that updated files
// replace the old entry in the retained index snapshot.
func TestSynchronizer_IndexedPushAppliesUpdateDelta(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockFS.Files["old.txt"] = domain.LocalFile{Path: "old.txt", Size: 200, Checksum: "newchecksum", ModTime: 999}

	mockStorage := NewMockBlobStorage()
	mockStorage.Indexes[1] = map[int64]*domain.FileIndex{2: {Entries: []domain.FileIndexEntry{
		{Path: "old.txt", Size: 100, MessageID: 100, Checksum: "oldchecksum", ModTime: 1},
	}}}
	mockStorage.IndexIDs[1] = map[int64]int{2: 500}

	sync := NewSynchronizer(mockFS, mockStorage, 1, &MockUserInterface{Confirmed: true}, false)
	if err := sync.Push(context.Background(), "/tmp", 1, 2); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	index := mockStorage.Indexes[1][2]
	if index == nil {
		t.Fatal("expected updated index")
	}
	var found bool
	for _, e := range index.Entries {
		if e.Path == "old.txt" {
			found = true
			if e.Size != 200 {
				t.Errorf("expected updated size 200, got %d", e.Size)
			}
		}
	}
	if !found {
		t.Errorf("expected old.txt to be present in delta-updated index")
	}
}

// TestSynchronizer_IndexedPushDeletesOldIndexMessage verifies that the previous
// known index message ID is deleted by ID, not discovered via a topic scan.
func TestSynchronizer_IndexedPushDeletesOldIndexMessage(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockFS.Files["new.txt"] = domain.LocalFile{Path: "new.txt", Size: 50}

	mockStorage := NewMockBlobStorage()
	mockStorage.Indexes[1] = map[int64]*domain.FileIndex{2: {Entries: []domain.FileIndexEntry{
		{Path: "old.txt", Size: 100, MessageID: 100},
	}}}
	mockStorage.IndexIDs[1] = map[int64]int{2: 500}

	sync := NewSynchronizer(mockFS, mockStorage, 1, &MockUserInterface{Confirmed: true}, false)
	if err := sync.Push(context.Background(), "/tmp", 1, 2); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// The old index message ID 500 must be among deleted IDs
	var found bool
	for _, id := range mockStorage.DeletedIDs {
		if id == 500 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected old index message ID 500 to be deleted, deleted IDs: %v", mockStorage.DeletedIDs)
	}
}

// TestSynchronizer_LegacyPushStillRebuildsFromFullScan verifies that legacy
// topics (no index at scan time) still use the full-scan rebuild path.
func TestSynchronizer_LegacyPushStillRebuildsFromFullScan(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockFS.Files["new.txt"] = domain.LocalFile{Path: "new.txt", Size: 50}

	mockStorage := NewMockBlobStorage()
	// No index - legacy topic
	mockStorage.Files[1] = map[int64][]domain.RemoteFile{2: {
		{Meta: domain.FileMeta{Path: "old.txt"}, Size: 100, MessageID: 100},
	}}

	sync := NewSynchronizer(mockFS, mockStorage, 1, &MockUserInterface{Confirmed: true}, false)
	if err := sync.Push(context.Background(), "/tmp", 1, 2); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// Legacy path should call ListFiles during migration and rebuild
	if mockStorage.ListFilesCalls == 0 {
		t.Errorf("expected ListFiles to be called for legacy topic")
	}
}
