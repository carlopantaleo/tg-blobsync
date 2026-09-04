package ui

import (
	"testing"
	"tg-blobsync/internal/domain"
)

func TestConsoleUI_HeadlessProgress(t *testing.T) {
	ui := NewConsoleUITest()
	ui.SetTotalFiles(2)

	task := ui.Start("one.txt", 100)
	task.Increment(10)
	task.SetCurrent(100)
	task.Complete()

	if ui.completedFiles != 1 {
		t.Fatalf("completedFiles = %d, want 1", ui.completedFiles)
	}
	task2 := ui.Start("two.txt", 50)
	task2.Abort()

	if ui.completedFiles != 1 {
		t.Fatalf("completedFiles after abort = %d, want 1", ui.completedFiles)
	}
}

func TestConsoleUI_ChunkProgress(t *testing.T) {
	ui := NewConsoleUITest()
	task := ui.Start("large.bin", 100)
	task.SetChunk(2, 5)
	consoleTask := ui.activeTasks["large.bin"]
	if consoleTask.chunkCurrent != 2 || consoleTask.chunkTotal != 5 {
		t.Fatalf("chunk progress = %d/%d, want 2/5", consoleTask.chunkCurrent, consoleTask.chunkTotal)
	}
}

func TestConsoleUI_WaitProgress(t *testing.T) {
	ui := NewConsoleUITest()
	ui.SetTotalFiles(1)
	task := ui.Start("f", 10)
	task.SetCurrent(10)
	task.Complete()
	ui.Wait()
}

func TestConsoleUI_Interactive(t *testing.T) {
	ui := NewConsoleUI()

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

	// Minimal checks that start/complete/abort work without panics
	uiInteractive := NewConsoleUI()
	taskInteractive := uiInteractive.Start("interactive.txt", 100)
	taskInteractive.Increment(10)
	taskInteractive.SetCurrent(20)
	taskInteractive.Complete()
	taskInteractive2 := uiInteractive.Start("interactive_abort.txt", 100)
	taskInteractive2.Abort()
}

func TestConsoleUI_BuildBrowserItems(t *testing.T) {
	ui := NewConsoleUITest()
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

	// Expected: [Back to Groups], root.txt, dir/, Exit Browser
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

	// Expected: [Back to Groups], .. [Go Up], subdir/, file1.txt, Exit Browser
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

	ui := NewConsoleUITest()
	go func() {
		ui.tuiModel.responseChan <- listItem{title: "back", value: "back"}
	}()
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

func TestConsoleUI_SelectSession_Headless(t *testing.T) {
	ui := NewConsoleUITest()
	sessions := []domain.SessionInfo{{ID: "s1"}, {ID: "s2", IsActive: true}}
	go func() {
		ui.tuiModel.responseChan <- listItem{value: "s2"}
	}()

	id, err := ui.SelectSession(sessions)
	if err != nil {
		t.Fatalf("SelectSession error: %v", err)
	}
	if id != "s2" {
		t.Fatalf("got %s, want s2", id)
	}
}

func TestConsoleUI_ConfirmDeleteSession_Headless(t *testing.T) {
	ui := NewConsoleUITest()
	go func() {
		ui.tuiModel.responseChan <- listItem{value: true}
	}()
	ok, err := ui.ConfirmDeleteSession(domain.SessionInfo{ID: "s1"})
	if err != nil || !ok {
		t.Fatalf("ConfirmDeleteSession got (%v, %v), want (true, nil)", ok, err)
	}
}

func TestConsoleUI_SelectSessionAction_Headless(t *testing.T) {
	ui := NewConsoleUITest()
	go func() {
		ui.tuiModel.responseChan <- listItem{value: "delete"}
	}()
	action, err := ui.SelectSessionAction()
	if err != nil || action != "delete" {
		t.Fatalf("SelectSessionAction got (%s, %v), want (delete, nil)", action, err)
	}
}

func TestConsoleUI_Prompts_Headless(t *testing.T) {
	ui := NewConsoleUITest()
	go func() {
		ui.tuiModel.responseChan <- "+39000000000"
	}()
	phone, err := ui.GetPhoneNumber()
	if err != nil || phone != "+39000000000" {
		t.Fatalf("GetPhoneNumber got (%s, %v)", phone, err)
	}

	go func() { ui.tuiModel.responseChan <- "12345" }()
	code, err := ui.GetCode()
	if err != nil || code != "12345" {
		t.Fatalf("GetCode got (%s, %v)", code, err)
	}

	go func() { ui.tuiModel.responseChan <- "pwd" }()
	pwd, err := ui.GetPassword()
	if err != nil || pwd != "pwd" {
		t.Fatalf("GetPassword got (%s, %v)", pwd, err)
	}
}

func TestConsoleUI_SelectGroupTopicSubDir_Headless(t *testing.T) {
	ui := NewConsoleUITest()
	groups := []domain.Group{{ID: 1, Title: "G"}}
	go func() { ui.tuiModel.responseChan <- listItem{value: groups[0]} }()
	g, err := ui.SelectGroup(groups)
	if err != nil || g.ID != 1 {
		t.Fatalf("SelectGroup got (%v, %v)", g, err)
	}

	topics := []domain.Topic{{ID: 2, Title: "T"}}
	go func() { ui.tuiModel.responseChan <- listItem{value: topics[0]} }()
	topic, err := ui.SelectTopic(topics)
	if err != nil || topic.ID != 2 {
		t.Fatalf("SelectTopic got (%v, %v)", topic, err)
	}

	go func() {
		ui.tuiModel.responseChan <- listItem{value: SubDirSelection{Action: SubDirCustom}}
		ui.tuiModel.responseChan <- "sub/path"
	}()
	sel, err := ui.SelectSubDir([]string{"existing"}, "")
	if err != nil || sel.Action != SubDirCustom || sel.Value != "sub/path" {
		t.Fatalf("SelectSubDir got (%+v, %v)", sel, err)
	}
}

func TestConsoleUI_SelectSubDirDrillDown_Headless(t *testing.T) {
	ui := NewConsoleUITest()

	// Enter a child directory from inside "a"
	go func() { ui.tuiModel.responseChan <- listItem{value: SubDirSelection{Action: SubDirEnter, Value: "b"}} }()
	sel, err := ui.SelectSubDir([]string{"b"}, "a")
	if err != nil || sel.Action != SubDirEnter || sel.Value != "b" {
		t.Fatalf("SelectSubDir enter got (%+v, %v)", sel, err)
	}

	// Confirm the current directory
	go func() { ui.tuiModel.responseChan <- listItem{value: SubDirSelection{Action: SubDirThis}} }()
	sel, err = ui.SelectSubDir([]string{"c"}, "a/b")
	if err != nil || sel.Action != SubDirThis {
		t.Fatalf("SelectSubDir this got (%+v, %v)", sel, err)
	}

	// Go up from the root level
	go func() { ui.tuiModel.responseChan <- listItem{value: SubDirSelection{Action: SubDirUp}} }()
	sel, err = ui.SelectSubDir([]string{"a"}, "")
	if err != nil || sel.Action != SubDirUp {
		t.Fatalf("SelectSubDir up got (%+v, %v)", sel, err)
	}

	// Enter custom path relative to the current directory
	go func() {
		ui.tuiModel.responseChan <- listItem{value: SubDirSelection{Action: SubDirCustom}}
		ui.tuiModel.responseChan <- "x/y"
	}()
	sel, err = ui.SelectSubDir(nil, "a/b")
	if err != nil || sel.Action != SubDirCustom || sel.Value != "x/y" {
		t.Fatalf("SelectSubDir custom got (%+v, %v)", sel, err)
	}
}

func TestConsoleUI_PromptAndBrowseFiles_Headless(t *testing.T) {
	ui := NewConsoleUITest()
	go func() { ui.tuiModel.responseChan <- "hello" }()
	val, err := ui.Prompt("label")
	if err != nil || val != "hello" {
		t.Fatalf("Prompt got (%s, %v)", val, err)
	}

	// BrowseFiles with no files should return nil without error and update content safely
	res, err := ui.BrowseFiles(nil)
	if err != nil || res != nil {
		t.Fatalf("BrowseFiles got (%v, %v), want (nil, nil)", res, err)
	}
}

func TestConsoleUI_WaitAndClose(t *testing.T) {
	ui := NewConsoleUITest()
	go func() { ui.tuiModel.responseChan <- "" }()
	if err := ui.WaitForInput("msg"); err != nil {
		t.Fatalf("WaitForInput err: %v", err)
	}
	ui.Wait()
	ui.Close()
}

func TestConsoleUI_ConfirmSync_Paths(t *testing.T) {
	ui := NewConsoleUITest()
	plan := domain.SyncPlan{Items: []domain.SyncItem{{Path: "a", Action: domain.ActionUpload}}}
	// details then cancel
	go func() {
		ui.tuiModel.responseChan <- listItem{value: "details"}
		ui.tuiModel.responseChan <- listItem{value: "back"}
		ui.tuiModel.responseChan <- listItem{value: "cancel"}
	}()
	ok, err := ui.ConfirmSync(plan)
	if err != nil || ok {
		t.Fatalf("ConfirmSync details->cancel got (%v,%v), want (false,nil)", ok, err)
	}
	// start path
	go func() { ui.tuiModel.responseChan <- listItem{value: "start"} }()
	ok, err = ui.ConfirmSync(plan)
	if err != nil || !ok {
		t.Fatalf("ConfirmSync start got (%v,%v), want (true,nil)", ok, err)
	}
}

func TestConsoleUI_PromptInt(t *testing.T) {
	ui := NewConsoleUITest()
	go func() { ui.tuiModel.responseChan <- "42" }()
	val, err := ui.PromptInt("num")
	if err != nil || val != 42 {
		t.Fatalf("PromptInt got (%d,%v)", val, err)
	}
}

func TestConsoleUI_BrowseFiles_BackAndDownload(t *testing.T) {
	file := domain.RemoteFile{Meta: domain.FileMeta{Path: "file.txt", ModTime: 1}, Size: 10, MessageID: 1}
	ui := NewConsoleUITest()
	// back path
	go func() { ui.tuiModel.responseChan <- listItem{value: browserMenuEntry{Label: ".. [Back to Groups]"}} }()
	_, err := ui.BrowseFiles([]domain.RemoteFile{file})
	if err == nil || err.Error() != "back" {
		t.Fatalf("BrowseFiles back err = %v", err)
	}
	// download path
	ui2 := NewConsoleUITest()
	go func() {
		ui2.tuiModel.responseChan <- listItem{value: browserMenuEntry{Label: "file", File: &file}}
		ui2.tuiModel.responseChan <- listItem{value: "download"}
	}()
	res, err := ui2.BrowseFiles([]domain.RemoteFile{file})
	if err != nil {
		t.Fatalf("BrowseFiles download err: %v", err)
	}
	if _, ok := res.(*domain.DownloadRequest); !ok {
		t.Fatalf("BrowseFiles download res type %T", res)
	}
}

func TestConsoleUI_ShowSessions_NoPanic(t *testing.T) {
	ui := NewConsoleUITest()
	ui.ShowSessions([]domain.SessionInfo{{ID: "a", IsActive: true}})
}

func TestListItemAndWriterHelpers(t *testing.T) {
	item := listItem{title: "t", desc: "d"}
	if item.Title() != "t" || item.Description() != "d" || item.FilterValue() != "t" {
		t.Fatal("listItem helper methods failed")
	}
	w := &TUIWriter{}
	if n, err := w.Write([]byte("log")); err != nil || n != 3 {
		t.Fatalf("TUIWriter Write got (%d,%v)", n, err)
	}
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
