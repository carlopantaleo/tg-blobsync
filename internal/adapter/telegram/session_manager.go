package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"tg-blobsync/internal/domain"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
)

type SessionManager struct {
	appID      int
	appHash    string
	sessionDir string
	ui         domain.UserInterface
}

type sessionMetadata struct {
	ActiveID string `json:"active_id"`
}

func NewSessionManager(appID int, appHash string, sessionDir string, ui domain.UserInterface) *SessionManager {
	return &SessionManager{
		appID:      appID,
		appHash:    appHash,
		sessionDir: sessionDir,
		ui:         ui,
	}
}

func (s *SessionManager) ListSessions() ([]domain.SessionInfo, error) {
	files, err := os.ReadDir(s.sessionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	meta, _ := s.loadMetadata()
	var sessions []domain.SessionInfo

	for _, f := range files {
		if f.IsDir() || !strings.HasPrefix(f.Name(), "session_") || !strings.HasSuffix(f.Name(), ".json") || f.Name() == "metadata.json" {
			continue
		}

		id := strings.TrimPrefix(strings.TrimSuffix(f.Name(), ".json"), "session_")
		sessions = append(sessions, domain.SessionInfo{
			ID:       id,
			Username: id, // For now, use ID as username until we can extract it from session
			IsActive: id == meta.ActiveID,
		})
	}

	return sessions, nil
}

func (s *SessionManager) AddSession(ctx context.Context) (domain.SessionInfo, error) {
	// Generate a temporary unique ID for the new session
	tempID := fmt.Sprintf("temp_%d", time.Now().Unix())
	sessionPath := filepath.Join(s.sessionDir, "session_"+tempID+".json")

	opts := telegram.Options{
		SessionStorage: &session.FileStorage{Path: sessionPath},
	}
	client := telegram.NewClient(s.appID, s.appHash, opts)

	var finalID string
	err := client.Run(ctx, func(ctx context.Context) error {
		flow := auth.NewFlow(termAuth{input: s.ui}, auth.SendCodeOptions{})
		if err := client.Auth().IfNecessary(ctx, flow); err != nil {
			return err
		}

		self, err := client.Self(ctx)
		if err != nil {
			return err
		}

		// Rename session file to username if available
		newID := self.Username
		if newID == "" {
			newID = fmt.Sprintf("%d", self.ID)
		}

		newPath := filepath.Join(s.sessionDir, "session_"+newID+".json")
		if err := os.Rename(sessionPath, newPath); err != nil {
			return err
		}
		finalID = newID
		return nil
	})

	if err != nil {
		return domain.SessionInfo{}, err
	}

	info := domain.SessionInfo{ID: finalID, Username: finalID, IsActive: true}
	s.SelectSession(finalID)
	return info, nil
}

func (s *SessionManager) SelectSession(id string) error {
	meta := sessionMetadata{ActiveID: id}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.sessionDir, "metadata.json"), data, 0600)
}

func (s *SessionManager) DeleteSession(id string) error {
	sessionPath := filepath.Join(s.sessionDir, "session_"+id+".json")
	if err := os.Remove(sessionPath); err != nil {
		return err
	}

	meta, _ := s.loadMetadata()
	if meta.ActiveID == id {
		meta.ActiveID = ""
		data, _ := json.Marshal(meta)
		os.WriteFile(filepath.Join(s.sessionDir, "metadata.json"), data, 0600)
	}
	return nil
}

func (s *SessionManager) GetActiveSession() (*domain.SessionInfo, error) {
	meta, err := s.loadMetadata()
	if err != nil || meta.ActiveID == "" {
		return nil, nil
	}

	return &domain.SessionInfo{
		ID:       meta.ActiveID,
		Username: meta.ActiveID,
		IsActive: true,
	}, nil
}

func (s *SessionManager) loadMetadata() (sessionMetadata, error) {
	var meta sessionMetadata
	data, err := os.ReadFile(filepath.Join(s.sessionDir, "metadata.json"))
	if err != nil {
		return meta, err
	}
	err = json.Unmarshal(data, &meta)
	return meta, err
}
