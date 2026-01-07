package domain

import (
	"context"
	"io"
)

// BlobStorage defines the interface for interacting with the remote storage (Telegram).
type BlobStorage interface {
	// Auth & Selection
	ListGroups(ctx context.Context) ([]Group, error)
	ListTopics(ctx context.Context, groupID int64) ([]Topic, error)

	// File Operations
	ListFiles(ctx context.Context, groupID int64, topicID int64) ([]RemoteFile, error)
	UploadFile(ctx context.Context, groupID int64, topicID int64, file LocalFile, data io.Reader) error
	DeleteFile(ctx context.Context, groupID int64, topicID int64, messageID int) error
	DownloadFile(ctx context.Context, groupID int64, topicID int64, messageID int, fileName string, size int64) (io.ReadCloser, error)

	// Lifecycle
	Close() error
}

// FileSystem defines the interface for interacting with the local filesystem.
type FileSystem interface {
	ListFiles(root string) ([]LocalFile, error)
	ReadFile(path string) (io.ReadCloser, error)
	WriteFile(path string, data io.Reader) error
	DeleteFile(path string) error
	EnsureDir(path string) error
}
