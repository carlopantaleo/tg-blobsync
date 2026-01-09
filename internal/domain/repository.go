package domain

import (
	"context"
	"io"
)

// ProgressTracker defines the interface for tracking file transfer progress.
type ProgressTracker interface {
	SetTotalFiles(total int)
	Start(name string, total int64) ProgressTask
	Wait()
}

// SyncConfirmer defines the interface for confirming synchronization plans.
type SyncConfirmer interface {
	ConfirmSync(plan SyncPlan) (bool, error)
}

// UserInterface combines progress tracking and confirmation.
type UserInterface interface {
	ProgressTracker
	SyncConfirmer
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
	SetProgressTracker(tracker ProgressTracker)
}

// FileSystem defines the interface for interacting with the local filesystem.
type FileSystem interface {
	ListFiles(root string, skipMD5 bool) ([]LocalFile, error)
	ReadFile(path string) (io.ReadCloser, error)
	WriteFile(path string, data io.Reader) error
	SetModTime(path string, modTime int64) error
	DeleteFile(path string) error
	EnsureDir(path string) error
}
