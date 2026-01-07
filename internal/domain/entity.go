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
