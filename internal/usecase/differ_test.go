package usecase

import (
	"testing"
	"tg-blobsync/internal/domain"
)

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
	// new.txt -> UPLOAD
	// changed.txt -> UPLOAD
	// same.txt -> nothing
	// deleted.txt -> DELETE_REMOTE

	if plan.Summary.ToUpload != 1 {
		t.Errorf("Expected 1 upload (new), got %d", plan.Summary.ToUpload)
	}
	if plan.Summary.ToUpdate != 1 {
		t.Errorf("Expected 1 update, got %d", plan.Summary.ToUpdate)
	}
	if plan.Summary.ToDelete != 1 {
		t.Errorf("Expected 1 delete, got %d", plan.Summary.ToDelete)
	}

	// Verify items
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
	// new.txt -> DOWNLOAD (New remote file)
	// same.txt -> nothing
	// changed.txt -> DOWNLOAD (Changed remote - ModTime diff)
	// deleted.txt -> DELETE_LOCAL (Deleted remotely)

	if plan.Summary.ToDownload != 1 {
		t.Errorf("Expected 1 download (new), got %d", plan.Summary.ToDownload)
	}
	if plan.Summary.ToUpdate != 1 {
		t.Errorf("Expected 1 update, got %d", plan.Summary.ToUpdate)
	}
	if plan.Summary.ToDelete != 1 {
		t.Errorf("Expected 1 delete local, got %d", plan.Summary.ToDelete)
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
