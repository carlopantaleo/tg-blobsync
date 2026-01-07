package config

import (
	"os"
	"path/filepath"
)

// AppConfig holds the application configuration.
type AppConfig struct {
	AppID      int
	AppHash    string
	SessionDir string
}

// GetSessionPath returns the path to the session file.
func GetSessionPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	sessionDir := filepath.Join(home, ".tg_blobsync")

	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return "", err
	}

	return filepath.Join(sessionDir, "session.json"), nil
}
