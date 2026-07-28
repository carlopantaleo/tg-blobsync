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

// ProgressTask defines the interface for tracking file transfer progress tasks.
type ProgressTask interface {
	Increment(n int)
	SetCurrent(current int64)
	SetChunk(current, total int)
	Complete()
	Abort()
}

// AuthInput defines an interface for interactive authentication input.
type AuthInput interface {
	GetPhoneNumber() (string, error)
	GetCode() (string, error)
	GetPassword() (string, error)
}

// SyncConfirmer defines the interface for confirming synchronization plans.
type SyncConfirmer interface {
	ConfirmSync(plan SyncPlan) (bool, error)
}

// SessionInfo represents an authentication session.
type SessionInfo struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

// SessionManager defines the interface for managing multiple authentication sessions.
type SessionManager interface {
	ListSessions() ([]SessionInfo, error)
	AddSession(ctx context.Context) (SessionInfo, error)
	SelectSession(id string) error
	DeleteSession(id string) error
	GetActiveSession() (*SessionInfo, error)
}

// UserInterface combines progress tracking, confirmation and session management interaction.
type UserInterface interface {
	ProgressTracker
	SyncConfirmer
	AuthInput
	SessionSelector
	TerminalInteractive
}

type TerminalInteractive interface {
	WaitForInput(message string) error
}

type SessionSelector interface {
	SelectSession(sessions []SessionInfo) (string, error)
	ConfirmDeleteSession(session SessionInfo) (bool, error)
	ShowSessions(sessions []SessionInfo)
	SelectSessionAction() (string, error)
}

// BlobStorage defines the interface for interacting with the remote storage (Telegram).
type BlobStorage interface {
	// Auth & Selection
	Start(ctx context.Context) error
	ListGroups(ctx context.Context) ([]Group, error)
	ListTopics(ctx context.Context, groupID int64) ([]Topic, error)
	GroupTotals(ctx context.Context, groupID int64) (GroupTotals, error)

	// File Operations
	ListFiles(ctx context.Context, groupID int64, topicID int64) ([]RemoteFile, error)
	GetIndex(ctx context.Context, groupID int64, topicID int64) (*FileIndex, int, bool, error)
	UploadIndex(ctx context.Context, groupID int64, topicID int64, index FileIndex) (int, error)
	ListIndexMessageIDs(ctx context.Context, groupID int64, topicID int64) ([]int, error)
	UploadFile(ctx context.Context, groupID int64, topicID int64, file LocalFile) ([]int, error)
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

type DownloadRequest struct {
	File RemoteFile
}

type NavigationError struct {
	Type string
	Data interface{}
}

func (e *NavigationError) Error() string {
	return e.Type
}
