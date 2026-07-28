package usecase

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"tg-blobsync/internal/domain"
)

// recordingTask records progress calls for assertion.
type recordingTask struct {
	name      string
	total     int64
	mu        sync.Mutex
	sets      []int64
	chunks    [][2]int
	completed bool
	aborted   bool
}

func (t *recordingTask) Increment(n int)               {}
func (t *recordingTask) SetCurrent(current int64)      { t.mu.Lock(); t.sets = append(t.sets, current); t.mu.Unlock() }
func (t *recordingTask) SetChunk(current, total int)   { t.mu.Lock(); t.chunks = append(t.chunks, [2]int{current, total}); t.mu.Unlock() }
func (t *recordingTask) Complete()                     { t.mu.Lock(); t.completed = true; t.mu.Unlock() }
func (t *recordingTask) Abort()                        { t.mu.Lock(); t.aborted = true; t.mu.Unlock() }

// recordingUI records progress task creation and total file counts.
type recordingUI struct {
	mu            sync.Mutex
	totalFiles    int
	tasks         []*recordingTask
	completedFile int
	confirmed     bool
}

func newRecordingUI() *recordingUI {
	return &recordingUI{confirmed: true}
}

func (u *recordingUI) ConfirmSync(plan domain.SyncPlan) (bool, error) { return u.confirmed, nil }
func (u *recordingUI) SetTotalFiles(total int)                        { u.mu.Lock(); u.totalFiles = total; u.mu.Unlock() }
func (u *recordingUI) Start(name string, total int64) domain.ProgressTask {
	t := &recordingTask{name: name, total: total}
	u.mu.Lock()
	u.tasks = append(u.tasks, t)
	u.mu.Unlock()
	return t
}
func (u *recordingUI) Wait() {}
func (u *recordingUI) GetPhoneNumber() (string, error) { return "", nil }
func (u *recordingUI) GetCode() (string, error)        { return "", nil }
func (u *recordingUI) GetPassword() (string, error)    { return "", nil }
func (u *recordingUI) SelectSession(sessions []domain.SessionInfo) (string, error) {
	return "", nil
}
func (u *recordingUI) ConfirmDeleteSession(session domain.SessionInfo) (bool, error) {
	return true, nil
}
func (u *recordingUI) ShowSessions(sessions []domain.SessionInfo) {}
func (u *recordingUI) SelectSessionAction() (string, error)       { return "exit", nil }
func (u *recordingUI) WaitForInput(message string) error          { return nil }

// TestExecutor_ChunkedPullProgressContributesOneLogicalFile verifies that a
// chunked pull contributes one logical file to totals and exposes per-chunk
// progress (current chunk index and total chunk count).
func TestExecutor_ChunkedPullProgressContributesOneLogicalFile(t *testing.T) {
	storage := NewMockBlobStorage()
	fs := NewMockFileSystem()
	ui := newRecordingUI()
	executor := NewExecutor(fs, storage, 1, ui)

	remote := domain.RemoteFile{
		Meta:      domain.FileMeta{Path: "large.bin"},
		ChunkIDs:  []int{10, 11, 12},
		Size:      30,
		MessageID: 10,
	}
	plan := domain.SyncPlan{
		Items: []domain.SyncItem{{
			Path:       "large.bin",
			Action:     domain.ActionDownload,
			RemoteFile: &remote,
		}},
		Summary: domain.SyncSummary{Total: 1, ToDownload: 1},
	}

	if _, err := executor.Execute(context.Background(), plan, "root", 1, 2); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Total files should be 1 (logical file), not 3 (chunks)
	if ui.totalFiles != 1 {
		t.Errorf("totalFiles = %d, want 1 (logical file)", ui.totalFiles)
	}

	// Should create exactly one progress task for the logical file
	if len(ui.tasks) != 1 {
		t.Fatalf("expected 1 progress task, got %d", len(ui.tasks))
	}

	task := ui.tasks[0]
	if task.name != "large.bin" {
		t.Errorf("task name = %s, want large.bin", task.name)
	}

	// Should report per-chunk progress with current chunk index and total chunks
	if len(task.chunks) == 0 {
		t.Errorf("expected per-chunk progress calls, got none")
	}
	// The last chunk call should indicate the final chunk
	last := task.chunks[len(task.chunks)-1]
	if last[1] != 3 {
		t.Errorf("total chunks = %d, want 3", last[1])
	}

	// The task should be completed
	if !task.completed {
		t.Errorf("expected task to be completed")
	}
}

// TestExecutor_NonChunkedPullProgressUnchanged verifies that non-chunked pull
// progress behavior remains the same (one task, no chunk calls).
func TestExecutor_NonChunkedPullProgressUnchanged(t *testing.T) {
	storage := NewMockBlobStorage()
	fs := NewMockFileSystem()
	ui := newRecordingUI()
	executor := NewExecutor(fs, storage, 1, ui)

	remote := domain.RemoteFile{
		Meta:      domain.FileMeta{Path: "regular.txt"},
		Size:      100,
		MessageID: 5,
	}
	plan := domain.SyncPlan{
		Items: []domain.SyncItem{{
			Path:       "regular.txt",
			Action:     domain.ActionDownload,
			RemoteFile: &remote,
		}},
		Summary: domain.SyncSummary{Total: 1, ToDownload: 1},
	}

	if _, err := executor.Execute(context.Background(), plan, filepath.Join("tmp", "test"), 1, 2); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(ui.tasks) != 1 {
		t.Fatalf("expected 1 progress task, got %d", len(ui.tasks))
	}
	task := ui.tasks[0]
	if len(task.chunks) != 0 {
		t.Errorf("non-chunked download should not report chunk progress, got %d calls", len(task.chunks))
	}
	if !task.completed {
		t.Errorf("expected task to be completed")
	}
}
