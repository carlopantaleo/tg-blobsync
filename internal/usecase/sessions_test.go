package usecase

import (
	"context"
	"errors"
	"testing"
	"tg-blobsync/internal/domain"
)

type fakeSessionRepo struct {
	sessions   []domain.SessionInfo
	added      bool
	selectedID string
	deletedID  string
	listErr    error
	addErr     error
	selectErr  error
	deleteErr  error
}

func (f *fakeSessionRepo) ListSessions() ([]domain.SessionInfo, error) { return f.sessions, f.listErr }
func (f *fakeSessionRepo) AddSession(ctx context.Context) (domain.SessionInfo, error) {
	if f.addErr != nil {
		return domain.SessionInfo{}, f.addErr
	}
	f.added = true
	info := domain.SessionInfo{ID: "new", Username: "new", IsActive: true}
	f.sessions = append(f.sessions, info)
	return info, nil
}
func (f *fakeSessionRepo) SelectSession(id string) error {
	f.selectedID = id
	return f.selectErr
}
func (f *fakeSessionRepo) DeleteSession(id string) error {
	f.deletedID = id
	return f.deleteErr
}
func (f *fakeSessionRepo) GetActiveSession() (*domain.SessionInfo, error) { return nil, nil }

// fake UI

type fakeSessionUI struct {
	actions          []string
	sessionsShown    [][]domain.SessionInfo
	selectID         string
	confirmDelete    bool
	selectActionErr  error
	selectSessionErr error
	confirmErr       error
}

func (u *fakeSessionUI) SelectSessionAction() (string, error) {
	if len(u.actions) == 0 {
		return "exit", nil
	}
	a := u.actions[0]
	u.actions = u.actions[1:]
	return a, u.selectActionErr
}

func (u *fakeSessionUI) ShowSessions(s []domain.SessionInfo) {
	u.sessionsShown = append(u.sessionsShown, s)
}
func (u *fakeSessionUI) SelectSession([]domain.SessionInfo) (string, error) {
	return u.selectID, u.selectSessionErr
}
func (u *fakeSessionUI) ConfirmDeleteSession(domain.SessionInfo) (bool, error) {
	return u.confirmDelete, u.confirmErr
}

func TestSessionManagerFlows(t *testing.T) {
	repo := &fakeSessionRepo{sessions: []domain.SessionInfo{{ID: "a"}, {ID: "b"}}}
	ui := &fakeSessionUI{actions: []string{"select", "delete", "create", "exit"}, selectID: "a", confirmDelete: true}
	mgr := NewSessionManager(repo, ui)

	if err := mgr.Manage(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.selectedID != "a" {
		t.Fatalf("expected selected a, got %s", repo.selectedID)
	}
	if repo.deletedID != "a" {
		t.Fatalf("expected deleted a, got %s", repo.deletedID)
	}
	if !repo.added {
		t.Fatalf("expected AddSession to be called")
	}
}

func TestSessionManagerErrors(t *testing.T) {
	repo := &fakeSessionRepo{listErr: errors.New("boom")}
	ui := &fakeSessionUI{}
	mgr := NewSessionManager(repo, ui)

	if err := mgr.Manage(context.Background()); err == nil {
		t.Fatalf("expected error on list")
	}
}
