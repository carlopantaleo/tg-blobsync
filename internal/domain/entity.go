package domain

const (
	// EmptyFileFlag marks an empty file stored as a one-byte Telegram document.
	EmptyFileFlag = "EMPTY_FILE"
	// IndexFlag marks a Telegram message containing a topic metadata index.
	IndexFlag = "INDEX"
	// ChunkFlag marks a Telegram message containing one chunk of a logical file.
	ChunkFlag = "CHUNK"
)

// FileMeta represents the metadata stored in the caption of the Telegram message.
type FileMeta struct {
	Path     string `json:"p"`
	Checksum string `json:"m,omitempty"`
	ModTime  int64  `json:"t,omitempty"`
	Flags    string `json:"f,omitempty"`
	Idx      int    `json:"i,omitempty"`
}

// FileIndexEntry represents a remote file in a topic metadata index.
type FileIndexEntry struct {
	Path      string `json:"p"`
	Checksum  string `json:"m,omitempty"`
	ModTime   int64  `json:"t,omitempty"`
	Flags     string `json:"f,omitempty"`
	Size      int64  `json:"s"`
	MessageID int    `json:"id"`
	ChunkIDs  []int  `json:"chunkIDs,omitempty"`
}

// FileIndex represents the metadata index stored in the most recent topic message.
type FileIndex struct {
	Entries []FileIndexEntry `json:"entries"`
}

// NewFileIndex creates an index containing the supplied remote files.
func NewFileIndex(files []RemoteFile) FileIndex {
	entries := make([]FileIndexEntry, len(files))
	for i, file := range files {
		entries[i] = FileIndexEntry{
			Path:      file.Meta.Path,
			Checksum:  file.Meta.Checksum,
			ModTime:   file.Meta.ModTime,
			Flags:     file.Meta.Flags,
			Size:      file.Size,
			MessageID: file.MessageID,
			ChunkIDs:  append([]int(nil), file.ChunkIDs...),
		}
	}
	return FileIndex{Entries: entries}
}

// RemoteFiles returns the remote files represented by the index.
func (i FileIndex) RemoteFiles() []RemoteFile {
	files := make([]RemoteFile, len(i.Entries))
	for j, entry := range i.Entries {
		files[j] = RemoteFile{
			Meta: FileMeta{
				Path:     entry.Path,
				Checksum: entry.Checksum,
				ModTime:  entry.ModTime,
				Flags:    entry.Flags,
			},
			MessageID: entry.MessageID,
			Size:      entry.Size,
			ChunkIDs:  append([]int(nil), entry.ChunkIDs...),
		}
	}
	return files
}

// GroupTotals contains aggregated information for a group.
type GroupTotals struct {
	Files     int
	TotalSize int64
}

// RemoteFile represents a file stored on Telegram.
type RemoteFile struct {
	Meta      FileMeta
	MessageID int
	ChunkIDs  []int
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
	TotalSize  int64
}

// UploadedFile records the outcome of an upload operation for index delta updates.
type UploadedFile struct {
	Path      string
	Checksum  string
	ModTime   int64
	Flags     string
	Size      int64
	MessageID int
	ChunkIDs  []int
}

// DeletedFile records the path of a remote file that was deleted.
type DeletedFile struct {
	Path string
}

// SyncResult captures the operations completed during synchronization,
// enabling delta-based index updates without rereading the topic.
type SyncResult struct {
	Uploaded []UploadedFile
	Deleted  []DeletedFile
}
