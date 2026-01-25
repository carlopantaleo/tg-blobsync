package ui

import (
	"testing"
	"tg-blobsync/internal/domain"
)

func TestConsoleUI_NonInteractive(t *testing.T) {
	ui := NewConsoleUI(true)

	// Test SetTotalFiles
	ui.SetTotalFiles(10)
	if ui.totalFiles != 10 {
		t.Errorf("expected totalFiles 10, got %d", ui.totalFiles)
	}
	if ui.startedFiles != 0 || ui.completedFiles != 0 {
		t.Error("expected started and completed files to be reset to 0")
	}

	// Test Start
	task := ui.Start("test.txt", 100)
	if task == nil {
		t.Error("expected task, got nil")
	}
	if ui.startedFiles != 1 {
		t.Errorf("expected startedFiles 1, got %d", ui.startedFiles)
	}

	// Test Task methods
	task.SetCurrent(50)
	task.Increment(10)
	task.Complete()
	if ui.completedFiles != 1 {
		t.Errorf("expected completedFiles 1, got %d", ui.completedFiles)
	}

	// Test Abort
	task2 := ui.Start("abort.txt", 100)
	task2.Abort()
	// completedFiles should still be 1
	if ui.completedFiles != 1 {
		t.Errorf("expected completedFiles 1 after abort, got %d", ui.completedFiles)
	}

	// Test interactive mode tasks (minimal check to ensure no panic)
	uiInteractive := NewConsoleUI(false)
	taskInteractive := uiInteractive.Start("interactive.txt", 100)
	taskInteractive.Increment(10)
	taskInteractive.SetCurrent(20)
	taskInteractive.Complete()
	taskInteractive2 := uiInteractive.Start("interactive_abort.txt", 100)
	taskInteractive2.Abort()

	// Test ConfirmSync
	plan := domain.SyncPlan{
		Summary: domain.SyncSummary{Total: 1},
	}
	confirmed, err := ui.ConfirmSync(plan)
	if err != nil {
		t.Errorf("ConfirmSync error: %v", err)
	}
	if !confirmed {
		t.Error("ConfirmSync(NonInteractive) should return true")
	}
}

func TestConsoleUI_BuildBrowserItems(t *testing.T) {
	ui := NewConsoleUI(true)
	files := []domain.RemoteFile{
		{Meta: domain.FileMeta{Path: "root.txt"}, Size: 100},
		{Meta: domain.FileMeta{Path: "dir/file1.txt"}, Size: 200},
		{Meta: domain.FileMeta{Path: "dir/subdir/file2.txt"}, Size: 300},
	}

	// 1. Root Directory
	items, totalSize, err := ui.buildBrowserItems(files, "")
	if err != nil {
		t.Fatalf("buildBrowserItems(root) failed: %v", err)
	}

	if totalSize != 600 {
		t.Errorf("Total size mismatch: got %d, want 600", totalSize)
	}

	// Expected: [Back to Topics], root.txt, dir/, Exit Browser
	if len(items) != 4 {
		t.Errorf("Expected 4 items in root, got %d", len(items))
	}

	// Check content
	foundDir := false
	foundFile := false
	for _, item := range items {
		if item.IsDir && item.DirName == "dir" {
			foundDir = true
		}
		if !item.IsDir && item.File != nil && item.File.Meta.Path == "root.txt" {
			foundFile = true
		}
	}
	if !foundDir || !foundFile {
		t.Error("Missing directory 'dir' or file 'root.txt' in root listing")
	}

	// 2. Subdirectory 'dir'
	items, totalSize, err = ui.buildBrowserItems(files, "dir")
	if err != nil {
		t.Fatalf("buildBrowserItems(dir) failed: %v", err)
	}

	// Expected: [Back to Topics], .. [Go Up], subdir/, file1.txt, Exit Browser
	if len(items) != 5 {
		t.Errorf("Expected 5 items in 'dir', got %d", len(items))
	}

	foundUp := false
	foundSubDir := false
	foundSubFile := false

	for _, item := range items {
		if item.Label == ".. [Go Up]" {
			foundUp = true
		}
		if item.IsDir && item.DirName == "subdir" {
			foundSubDir = true
		}
		if !item.IsDir && item.File != nil && item.File.Meta.Path == "dir/file1.txt" {
			foundSubFile = true
		}
	}

	if !foundUp || !foundSubDir || !foundSubFile {
		t.Error("Missing items in subdirectory listing")
	}
}

func TestConsoleUI_ShowDetailedChanges(t *testing.T) {
	// This function prints to stdout. We verify it doesn't panic.
	// Capturing stdout is possible but often platform dependent or verbose in Go without helper libs.
	// Given the instructions, we can just ensure it runs.

	ui := NewConsoleUI(true)
	plan := domain.SyncPlan{
		Items: []domain.SyncItem{
			{Path: "up.txt", Action: domain.ActionUpload},
			{Path: "down.txt", Action: domain.ActionDownload},
			{Path: "del_rem.txt", Action: domain.ActionDeleteRemote},
			{Path: "del_loc.txt", Action: domain.ActionDeleteLocal},
			{Path: "skip.txt", Action: domain.ActionSkip, Reason: "exists"},
		},
	}

	ui.showDetailedChanges(plan)
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
	}

	for _, tt := range tests {
		res := formatSize(tt.input)
		if res != tt.expected {
			t.Errorf("formatSize(%d) = %s, want %s", tt.input, res, tt.expected)
		}
	}
}
