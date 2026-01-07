package domain

import (
	"context"
	"io"
)

// ProgressReporter defines the interface for reporting progress of file transfers.
type ProgressReporter interface {
	Start(name string, total int64) ProgressTask
	Wait()
}

type ProgressTask interface {
	Increment(n int)
	SetCurrent(current int64)
	Complete()
	Abort()
}

// BlobStorage defines the interface for interacting with the remote storage (Telegram).
type BlobStorage interface {
	// Auth & Selection
	ListGroups(ctx context.Context) ([]Group, error)
	ListTopics(ctx context.Context, groupID int64) ([]Topic, error)

	// File Operations
	ListFiles(ctx context.Context, groupID int64, topicID int64) ([]RemoteFile, error)
	UploadFile(ctx context.Context, groupID int64, topicID int64, file LocalFile) error
	DeleteFile(ctx context.Context, groupID int64, topicID int64, messageID int) error
	DownloadFile(ctx context.Context, groupID int64, topicID int64, messageID int, fileName string, size int64) (io.ReadCloser, error)

	// Lifecycle
	Close() error
	SetProgressReporter(reporter ProgressReporter)
}

// FileSystem defines the interface for interacting with the local filesystem.
type FileSystem interface {
	ListFiles(root string, skipMD5 bool) ([]LocalFile, error)
	ReadFile(path string) (io.ReadCloser, error)
	WriteFile(path string, data io.Reader) error
	DeleteFile(path string) error
	EnsureDir(path string) error
}
