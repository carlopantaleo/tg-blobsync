package telegram

import (
	"os"
	"path/filepath"
	"testing"
	"tg-blobsync/internal/domain"
)

type dummyUI struct{}

func (dummyUI) SetTotalFiles(int)                                     {}
func (dummyUI) Start(string, int64) domain.ProgressTask               { return nil }
func (dummyUI) Wait()                                                 {}
func (dummyUI) ConfirmSync(domain.SyncPlan) (bool, error)             { return true, nil }
func (dummyUI) GetPhoneNumber() (string, error)                       { return "", nil }
func (dummyUI) GetCode() (string, error)                              { return "", nil }
func (dummyUI) GetPassword() (string, error)                          { return "", nil }
func (dummyUI) WaitForInput(string) error                             { return nil }
func (dummyUI) SelectSession([]domain.SessionInfo) (string, error)    { return "", nil }
func (dummyUI) ConfirmDeleteSession(domain.SessionInfo) (bool, error) { return false, nil }
func (dummyUI) ShowSessions([]domain.SessionInfo)                     {}
func (dummyUI) SelectSessionAction() (string, error)                  { return "exit", nil }

// minimal ProgressTask implementation (never used)
type nopTask struct{}

func (nopTask) Increment(int)     {}
func (nopTask) SetCurrent(int64)  {}
func (nopTask) SetChunk(int, int) {}
func (nopTask) Complete()         {}
func (nopTask) Abort()            {}

func TestSessionManager_ListSelectDelete(t *testing.T) {
	dir := t.TempDir()
	// create two session files and metadata
	mustWrite(t, filepath.Join(dir, "session_a.json"), "{}")
	mustWrite(t, filepath.Join(dir, "session_b.json"), "{}")
	mustWrite(t, filepath.Join(dir, "metadata.json"), `{"active_id":"b"}`)

	mgr := NewSessionManager(1, "hash", dir, dummyUI{})

	sessions, err := mgr.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	var active string
	for _, s := range sessions {
		if s.IsActive {
			active = s.ID
		}
	}
	if active != "b" {
		t.Fatalf("expected active b, got %s", active)
	}

	// GetActiveSession reflects metadata
	info, err := mgr.GetActiveSession()
	if err != nil || info == nil || info.ID != "b" {
		t.Fatalf("GetActiveSession unexpected: %+v err=%v", info, err)
	}

	// SelectSession updates metadata
	if err := mgr.SelectSession("a"); err != nil {
		t.Fatalf("SelectSession: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if string(data) != "{\"active_id\":\"a\"}" {
		t.Fatalf("metadata not updated: %s", string(data))
	}

	// DeleteSession removes file and clears metadata if active
	if err := mgr.DeleteSession("a"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "session_a.json")); !os.IsNotExist(err) {
		t.Fatalf("session_a.json still exists")
	}
	// simulate active a then delete
	mustWrite(t, filepath.Join(dir, "metadata.json"), `{"active_id":"a"}`)
	mustWrite(t, filepath.Join(dir, "session_a.json"), "{}")
	if err := mgr.DeleteSession("a"); err != nil {
		t.Fatalf("DeleteSession active: %v", err)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "metadata.json"))
	if string(data) != "{\"active_id\":\"\"}" {
		t.Fatalf("metadata not cleared: %s", string(data))
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
