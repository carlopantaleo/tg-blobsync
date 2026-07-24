package usecase

import (
	"testing"
	"tg-blobsync/internal/domain"
)

func TestDiffer_TreatsChunkedFileAsSingleLogicalFile(t *testing.T) {
	differ := NewDiffer(true)
	local := map[string]domain.LocalFile{"large.bin": {Path: "large.bin", ModTime: 10, Size: 100}}
	remote := map[string]domain.RemoteFile{"large.bin": {
		Meta:     domain.FileMeta{Path: "large.bin", ModTime: 10, Flags: domain.ChunkFlag},
		ChunkIDs: []int{11, 12, 13},
		Size:     100,
	}}
	plan := differ.DiffPush(local, remote)
	if plan.Summary.Total != 0 {
		t.Fatalf("chunked file plan total = %d, want 0", plan.Summary.Total)
	}
}

func TestDiffer_SkipMD5UsesNormalizedEmptyFileSize(t *testing.T) {
	differ := NewDiffer(true)
	local := map[string]domain.LocalFile{"empty.txt": {Path: "empty.txt", ModTime: 10, Size: 0}}
	remote := map[string]domain.RemoteFile{"empty.txt": {Meta: domain.FileMeta{Path: "empty.txt", ModTime: 10, Flags: domain.EmptyFileFlag}, Size: 0}}

	plan := differ.DiffPush(local, remote)
	if plan.Summary.Total != 0 {
		t.Fatalf("empty file plan total = %d, want 0", plan.Summary.Total)
	}
}

func TestDiffer_DiffPush(t *testing.T) {
	differ := NewDiffer(false) // MD5 check enabled

	local := map[string]domain.LocalFile{
		"new.txt":     {Path: "new.txt", Checksum: "abc", ModTime: 100, Size: 10},
		"same.txt":    {Path: "same.txt", Checksum: "def", ModTime: 200, Size: 20},
		"changed.txt": {Path: "changed.txt", Checksum: "ghi-new", ModTime: 300, Size: 30},
	}

	remote := map[string]domain.RemoteFile{
		"same.txt":    {Meta: domain.FileMeta{Path: "same.txt", Checksum: "def", ModTime: 200}, Size: 20},
		"changed.txt": {Meta: domain.FileMeta{Path: "changed.txt", Checksum: "ghi-old", ModTime: 250}, Size: 30},
		"deleted.txt": {Meta: domain.FileMeta{Path: "deleted.txt", Checksum: "jkl", ModTime: 400}, Size: 40},
	}

	plan := differ.DiffPush(local, remote)

	// Expected:
	// changed.txt -> UPLOAD (30 bytes)
	// deleted.txt -> DELETE_REMOTE (40 bytes)
	// new.txt     -> UPLOAD (10 bytes)
	// Total size: 30 + 40 + 10 = 80 bytes

	if plan.Summary.ToUpload != 1 {
		t.Errorf("Expected 1 upload (new), got %d", plan.Summary.ToUpload)
	}
	if plan.Summary.ToUpdate != 1 {
		t.Errorf("Expected 1 update, got %d", plan.Summary.ToUpdate)
	}
	if plan.Summary.ToDelete != 1 {
		t.Errorf("Expected 1 delete, got %d", plan.Summary.ToDelete)
	}
	if plan.Summary.TotalSize != 80 {
		t.Errorf("Expected total size 80, got %d", plan.Summary.TotalSize)
	}

	// Verify items are sorted alphabetically
	expectedOrder := []string{"changed.txt", "deleted.txt", "new.txt"}
	if len(plan.Items) != len(expectedOrder) {
		t.Fatalf("Expected %d items, got %d", len(expectedOrder), len(plan.Items))
	}
	for i, item := range plan.Items {
		if item.Path != expectedOrder[i] {
			t.Errorf("Item at index %d: expected path %s, got %s", i, expectedOrder[i], item.Path)
		}
	}

	// Verify actions
	actions := make(map[string]domain.SyncActionType)
	for _, item := range plan.Items {
		actions[item.Path] = item.Action
	}

	if actions["new.txt"] != domain.ActionUpload {
		t.Errorf("new.txt action mismatch: %v", actions["new.txt"])
	}
	if actions["changed.txt"] != domain.ActionUpload {
		t.Errorf("changed.txt action mismatch: %v", actions["changed.txt"])
	}
	if actions["deleted.txt"] != domain.ActionDeleteRemote {
		t.Errorf("deleted.txt action mismatch: %v", actions["deleted.txt"])
	}
}

func TestDiffer_DiffPull(t *testing.T) {
	differ := NewDiffer(true) // SkipMD5 enabled (ModTime/Size check)

	local := map[string]domain.LocalFile{
		"same.txt":    {Path: "same.txt", ModTime: 200, Size: 20},
		"changed.txt": {Path: "changed.txt", ModTime: 250, Size: 30}, // Older/Different than remote
		"deleted.txt": {Path: "deleted.txt", ModTime: 400, Size: 40},
	}

	remote := map[string]domain.RemoteFile{
		"new.txt":     {Meta: domain.FileMeta{Path: "new.txt", ModTime: 100}, Size: 10},
		"same.txt":    {Meta: domain.FileMeta{Path: "same.txt", ModTime: 200}, Size: 20},
		"changed.txt": {Meta: domain.FileMeta{Path: "changed.txt", ModTime: 300}, Size: 30},
	}

	plan := differ.DiffPull(local, remote)

	// Expected:
	// changed.txt -> DOWNLOAD (Changed remote - 30 bytes)
	// deleted.txt -> DELETE_LOCAL (Deleted remotely - 40 bytes)
	// new.txt     -> DOWNLOAD (New remote file - 10 bytes)
	// Total size: 30 + 40 + 10 = 80 bytes

	if plan.Summary.ToDownload != 1 {
		t.Errorf("Expected 1 download (new), got %d", plan.Summary.ToDownload)
	}
	if plan.Summary.ToUpdate != 1 {
		t.Errorf("Expected 1 update, got %d", plan.Summary.ToUpdate)
	}
	if plan.Summary.ToDelete != 1 {
		t.Errorf("Expected 1 delete local, got %d", plan.Summary.ToDelete)
	}
	if plan.Summary.TotalSize != 80 {
		t.Errorf("Expected total size 80, got %d", plan.Summary.TotalSize)
	}

	// Verify items are sorted alphabetically
	expectedOrder := []string{"changed.txt", "deleted.txt", "new.txt"}
	if len(plan.Items) != len(expectedOrder) {
		t.Fatalf("Expected %d items, got %d", len(expectedOrder), len(plan.Items))
	}
	for i, item := range plan.Items {
		if item.Path != expectedOrder[i] {
			t.Errorf("Item at index %d: expected path %s, got %s", i, expectedOrder[i], item.Path)
		}
	}

	actions := make(map[string]domain.SyncActionType)
	for _, item := range plan.Items {
		actions[item.Path] = item.Action
	}

	if actions["new.txt"] != domain.ActionDownload {
		t.Errorf("new.txt action mismatch: %v", actions["new.txt"])
	}
	if actions["changed.txt"] != domain.ActionDownload {
		t.Errorf("changed.txt action mismatch: %v", actions["changed.txt"])
	}
	if actions["deleted.txt"] != domain.ActionDeleteLocal {
		t.Errorf("deleted.txt action mismatch: %v", actions["deleted.txt"])
	}
}
