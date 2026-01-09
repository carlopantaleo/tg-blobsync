package domain

// FileMeta represents the metadata stored in the caption of the Telegram message.
type FileMeta struct {
	Path     string `json:"p"`
	Checksum string `json:"m,omitempty"`
	ModTime  int64  `json:"t,omitempty"`
	Flags    string `json:"f,omitempty"`
}

// RemoteFile represents a file stored on Telegram.
type RemoteFile struct {
	Meta      FileMeta
	MessageID int
	Size      int64
}

// LocalFile represents a file on the local filesystem.
type LocalFile struct {
	Path     string // Relative path
	Checksum string
	ModTime  int64
	Size     int64
	AbsPath  string // Absolute path for internal use
}

// Group represents a Telegram Supergroup.
type Group struct {
	ID    int64
	Title string
}

// Topic represents a Telegram Forum Topic.
type Topic struct {
	ID    int64
	Title string
}

// SyncActionType defines the type of synchronization action.
type SyncActionType string

const (
	ActionUpload       SyncActionType = "UPLOAD"
	ActionDownload     SyncActionType = "DOWNLOAD"
	ActionDeleteRemote SyncActionType = "DELETE_REMOTE"
	ActionDeleteLocal  SyncActionType = "DELETE_LOCAL"
	ActionSkip         SyncActionType = "SKIP"
)

// SyncItem represents a single file synchronization task.
type SyncItem struct {
	Path       string
	Action     SyncActionType
	LocalFile  *LocalFile
	RemoteFile *RemoteFile
	Reason     string
}

// SyncPlan represents the complete set of actions to synchronize files.
type SyncPlan struct {
	Items   []SyncItem
	Summary SyncSummary
}

// SyncSummary contains the counts of actions in a plan.
type SyncSummary struct {
	ToUpload   int
	ToDownload int
	ToUpdate   int
	ToDelete   int
	Total      int
}
