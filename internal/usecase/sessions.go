package usecase

import (
	"context"
	"fmt"
	"tg-blobsync/internal/domain"
)

type SessionManager struct {
	repo domain.SessionManager
	ui   domain.SessionSelector
}

func NewSessionManager(repo domain.SessionManager, ui domain.SessionSelector) *SessionManager {
	return &SessionManager{
		repo: repo,
		ui:   ui,
	}
}

func (s *SessionManager) Manage(ctx context.Context) error {
	for {
		sessions, err := s.repo.ListSessions()
		if err != nil {
			return fmt.Errorf("failed to list sessions: %w", err)
		}

		s.ui.ShowSessions(sessions)

		action, err := s.promptAction()
		if err != nil {
			return err
		}

		switch action {
		case "create":
			_, err := s.repo.AddSession(ctx)
			if err != nil {
				return fmt.Errorf("failed to add session: %w", err)
			}
		case "select":
			if len(sessions) == 0 {
				fmt.Println("No sessions available to select.")
				continue
			}
			id, err := s.ui.SelectSession(sessions)
			if err != nil {
				return err
			}
			if err := s.repo.SelectSession(id); err != nil {
				return fmt.Errorf("failed to select session: %w", err)
			}
		case "delete":
			if len(sessions) == 0 {
				fmt.Println("No sessions available to delete.")
				continue
			}
			id, err := s.ui.SelectSession(sessions)
			if err != nil {
				return err
			}
			var sessionToDelete domain.SessionInfo
			for _, sess := range sessions {
				if sess.ID == id {
					sessionToDelete = sess
					break
				}
			}
			confirm, err := s.ui.ConfirmDeleteSession(sessionToDelete)
			if err != nil {
				return err
			}
			if confirm {
				if err := s.repo.DeleteSession(id); err != nil {
					return fmt.Errorf("failed to delete session: %w", err)
				}
			}
		case "exit":
			return nil
		}
	}
}

func (s *SessionManager) promptAction() (string, error) {
	return s.ui.SelectSessionAction()
}
