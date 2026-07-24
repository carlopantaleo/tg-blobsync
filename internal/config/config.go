package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultChunkThreshold int64 = 2 * 1024 * 1024 * 1024
	DefaultChunkSize      int64 = 1 * 1024 * 1024 * 1024
)

// AppConfig holds the application configuration.
type AppConfig struct {
	AppID          int
	AppHash        string
	SessionDir     string
	ChunkThreshold int64 `json:"chunkThreshold" yaml:"chunkThreshold"`
	ChunkSize      int64 `json:"chunkSize" yaml:"chunkSize"`
}

func ValidateChunkConfig(threshold, size int64) error {
	if threshold <= 0 {
		return fmt.Errorf("chunk threshold must be greater than zero")
	}
	if size <= 0 {
		return fmt.Errorf("chunk size must be greater than zero")
	}
	if size > threshold {
		return fmt.Errorf("chunk size must not exceed chunk threshold")
	}
	return nil
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
