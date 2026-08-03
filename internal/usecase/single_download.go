package usecase

import (
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"tg-blobsync/internal/domain"
)

// SingleDownloader downloads a single logical file (chunked or not) from a
// topic into a local destination directory. It is used by the browse flow to
// reconstruct chunked files selected by the user.
type SingleDownloader struct {
	fs      domain.FileSystem
	storage domain.BlobStorage
	ui      domain.UserInterface
}

// NewSingleDownloader creates a SingleDownloader backed by the supplied
// filesystem, storage, and UI progress tracker.
func NewSingleDownloader(fs domain.FileSystem, storage domain.BlobStorage, ui domain.UserInterface) *SingleDownloader {
	return &SingleDownloader{fs: fs, storage: storage, ui: ui}
}

// Download fetches the file described by req into destDir, streaming chunked
// files in order without buffering the whole file in memory.
func (d *SingleDownloader) Download(ctx context.Context, req *domain.DownloadRequest, destDir string, groupID, topicID int64) error {
	localPath := filepath.Join(destDir, req.File.Meta.Path)
	localDir := filepath.Dir(localPath)

	if err := d.fs.EnsureDir(localDir); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if d.ui != nil {
		d.ui.SetTotalFiles(1)
	}
	var task domain.ProgressTask
	if d.ui != nil {
		task = d.ui.Start(req.File.Meta.Path, req.File.Size)
	}

	reader, closers, err := d.openDownloadReader(ctx, req.File, groupID, topicID, task)
	if err != nil {
		if task != nil {
			task.Abort()
		}
		return err
	}
	defer closeReaders(closers)

	if err := d.fs.WriteFile(localPath, reader); err != nil {
		if task != nil {
			task.Abort()
		}
		return fmt.Errorf("failed to write file: %w", err)
	}

	if req.File.Meta.ModTime > 0 {
		if err := d.fs.SetModTime(localPath, req.File.Meta.ModTime); err != nil {
			log.Printf("Warning: failed to set modification time for %s: %v", localPath, err)
		}
	}

	if task != nil {
		task.Complete()
	}
	if d.ui != nil {
		d.ui.Wait()
	}
	return nil
}

func (d *SingleDownloader) openDownloadReader(ctx context.Context, file domain.RemoteFile, groupID, topicID int64, task domain.ProgressTask) (io.Reader, []io.Closer, error) {
	if file.Meta.Flags == domain.EmptyFileFlag {
		return io.NopCloser(newEmptyReader()), nil, nil
	}
	if len(file.ChunkIDs) > 0 {
		return d.openChunkedDownloadReader(ctx, file, groupID, topicID, task)
	}
	rc, err := d.storage.DownloadFile(ctx, groupID, topicID, file.MessageID, file.Meta.Path, file.Size, task)
	if err != nil {
		return nil, nil, fmt.Errorf("download failed: %w", err)
	}
	return rc, []io.Closer{rc}, nil
}

func (d *SingleDownloader) openChunkedDownloadReader(ctx context.Context, file domain.RemoteFile, groupID, topicID int64, task domain.ProgressTask) (io.Reader, []io.Closer, error) {
	if chunkedStorage, ok := d.storage.(chunkedDownloader); ok {
		rc, err := chunkedStorage.DownloadChunkedFile(ctx, groupID, topicID, file.ChunkIDs, file.Meta.Path, file.Size, task)
		if err != nil {
			return nil, nil, fmt.Errorf("download failed: %w", err)
		}
		return rc, []io.Closer{rc}, nil
	}

	readers := make([]io.Reader, 0, len(file.ChunkIDs))
	closers := make([]io.Closer, 0, len(file.ChunkIDs))
	for idx, messageID := range file.ChunkIDs {
		if task != nil {
			task.SetChunk(idx+1, len(file.ChunkIDs))
		}
		rc, err := d.storage.DownloadFile(ctx, groupID, topicID, messageID, file.Meta.Path, file.Size, task)
		if err != nil {
			closeReaders(closers)
			return nil, nil, fmt.Errorf("download chunk %d/%d failed: %w", idx+1, len(file.ChunkIDs), err)
		}
		readers = append(readers, rc)
		closers = append(closers, rc)
	}
	return io.MultiReader(readers...), closers, nil
}

type emptyReader struct{}

func newEmptyReader() *emptyReader { return &emptyReader{} }

func (e *emptyReader) Read(p []byte) (int, error) { return 0, io.EOF }
