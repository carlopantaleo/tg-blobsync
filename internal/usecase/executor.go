package usecase

import (
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"tg-blobsync/internal/domain"
	"tg-blobsync/internal/pkg/retry"
	"time"

	"golang.org/x/sync/errgroup"
)

type SyncExecutor interface {
	Execute(ctx context.Context, plan domain.SyncPlan, rootDir string, groupID, topicID int64) (domain.SyncResult, error)
	SetSubDir(subDir string)
}

type chunkedDownloader interface {
	DownloadChunkedFile(ctx context.Context, groupID, topicID int64, chunkIDs []int, fileName string, size int64) (io.ReadCloser, error)
}

type executor struct {
	fs      domain.FileSystem
	storage domain.BlobStorage
	workers int
	ui      domain.UserInterface
	subDir  string
	mu      sync.Mutex
	result  domain.SyncResult
}

func NewExecutor(fs domain.FileSystem, storage domain.BlobStorage, workers int, ui domain.UserInterface) SyncExecutor {
	if workers <= 0 {
		workers = 1
	}
	return &executor{
		fs:      fs,
		storage: storage,
		workers: workers,
		ui:      ui,
	}
}

func (e *executor) SetSubDir(subDir string) {
	e.subDir = subDir
}

func (e *executor) Execute(ctx context.Context, plan domain.SyncPlan, rootDir string, groupID, topicID int64) (domain.SyncResult, error) {
	if plan.Summary.Total == 0 {
		msg := "Everything is up to date."
		log.Println(msg)
		if e.ui != nil {
			_ = e.ui.WaitForInput(msg)
		}
		return domain.SyncResult{}, nil
	}

	// User Confirmation
	if e.ui != nil {
		confirmed, err := e.ui.ConfirmSync(plan)
		if err != nil {
			return domain.SyncResult{}, err
		}
		if !confirmed {
			log.Println("Sync cancelled by user.")
			return domain.SyncResult{}, nil
		}
	}

	if e.ui != nil {
		e.ui.SetTotalFiles(plan.Summary.Total)
	}

	// Separate Deletions from Transfer tasks
	var transferTasks []domain.SyncItem
	var deleteTasks []domain.SyncItem

	for _, item := range plan.Items {
		if item.Action == domain.ActionDeleteRemote || item.Action == domain.ActionDeleteLocal {
			deleteTasks = append(deleteTasks, item)
		} else {
			transferTasks = append(transferTasks, item)
		}
	}

	// Execute Transfers (Upload/Download)
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(e.workers)

	for _, item := range transferTasks {
		if gCtx.Err() != nil {
			break
		}

		item := item // capture loop var
		g.Go(func() error {
			return e.processItem(gCtx, item, rootDir, groupID, topicID)
		})
	}

	if err := g.Wait(); err != nil {
		return e.result, err
	}

	// Execute Deletions
	for _, item := range deleteTasks {
		if err := e.processItem(ctx, item, rootDir, groupID, topicID); err != nil {
			log.Printf("Error processing delete for %s: %v", item.Path, err)
		}
	}

	if e.ui != nil {
		e.ui.Wait()
		_ = e.ui.WaitForInput("Sync completed.")
	}

	return e.result, nil
}

func (e *executor) processItem(ctx context.Context, item domain.SyncItem, rootDir string, groupID, topicID int64) error {
	switch item.Action {
	case domain.ActionUpload:
		return e.upload(ctx, item, groupID, topicID)
	case domain.ActionDownload:
		return e.download(ctx, item, rootDir, groupID, topicID)
	case domain.ActionDeleteRemote:
		return e.deleteRemote(ctx, item, groupID, topicID)
	case domain.ActionDeleteLocal:
		return e.deleteLocal(item, rootDir)
	}
	return nil
}

func (e *executor) upload(ctx context.Context, item domain.SyncItem, groupID, topicID int64) error {
	if item.LocalFile == nil {
		return fmt.Errorf("local file is nil for upload: %s", item.Path)
	}

	// Prepare local file for upload with corrected remote path
	uploadFile := *item.LocalFile
	if e.subDir != "" {
		uploadFile.Path = filepath.ToSlash(filepath.Join(e.subDir, item.Path))
	} else {
		uploadFile.Path = item.Path
	}

	messageIDs, err := e.storage.UploadFile(ctx, groupID, topicID, uploadFile)
	if err != nil {
		return fmt.Errorf("error uploading file %s: %w", item.Path, err)
	}

	// Record the upload result for delta-based index updates
	flags := ""
	if uploadFile.Size == 0 {
		flags = domain.EmptyFileFlag
	}
	if len(messageIDs) > 1 {
		flags = domain.ChunkFlag
	}
	entry := domain.UploadedFile{
		Path:     uploadFile.Path,
		Checksum: uploadFile.Checksum,
		ModTime:  uploadFile.ModTime,
		Flags:    flags,
		Size:     uploadFile.Size,
	}
	if len(messageIDs) > 1 {
		entry.ChunkIDs = messageIDs
	} else if len(messageIDs) == 1 {
		entry.MessageID = messageIDs[0]
	}
	e.mu.Lock()
	e.result.Uploaded = append(e.result.Uploaded, entry)
	e.mu.Unlock()

	// If it was an update (RemoteFile exists), delete the old version on Telegram
	if item.RemoteFile != nil {
		log.Printf("[*] Deleting old version of: %s", item.Path)
		for _, messageID := range remoteMessageIDs(*item.RemoteFile) {
			if err := e.storage.DeleteFile(ctx, groupID, topicID, messageID); err != nil {
				log.Printf("Warning: failed to delete old version message %d of %s: %v", messageID, item.Path, err)
			}
		}
	}
	return nil
}

func (e *executor) download(ctx context.Context, item domain.SyncItem, rootDir string, groupID, topicID int64) error {
	if item.RemoteFile == nil {
		return fmt.Errorf("remote file is nil for download: %s", item.Path)
	}

	remoteFile := item.RemoteFile
	fullPath := filepath.Join(rootDir, item.Path)

	operation := func() error {
		if remoteFile.Meta.Flags == domain.EmptyFileFlag {
			log.Printf("[*] Restoring empty file: %s", item.Path)
			if err := e.fs.WriteFile(fullPath, strings.NewReader("")); err != nil {
				return fmt.Errorf("error creating empty file %s: %w", item.Path, err)
			}
			if err := e.fs.SetModTime(fullPath, remoteFile.Meta.ModTime); err != nil {
				log.Printf("Warning: failed to set modification time for %s: %v", item.Path, err)
			}
			return nil
		}

		var reader io.Reader
		var closers []io.Closer
		if len(remoteFile.ChunkIDs) > 0 {
			if chunkedStorage, ok := e.storage.(chunkedDownloader); ok {
				chunkReader, downloadErr := chunkedStorage.DownloadChunkedFile(ctx, groupID, topicID, remoteFile.ChunkIDs, remoteFile.Meta.Path, remoteFile.Size)
				if downloadErr != nil {
					return fmt.Errorf("error downloading file %s: %w", item.Path, downloadErr)
				}
				reader = chunkReader
				closers = append(closers, chunkReader)
			} else {
				var downloadErr error
				reader, closers, downloadErr = e.openChunkReaders(ctx, *remoteFile, groupID, topicID)
				if downloadErr != nil {
					return fmt.Errorf("error downloading file %s: %w", item.Path, downloadErr)
				}
			}
		} else {
			readers := make([]io.Reader, 0, 1)
			for _, messageID := range remoteMessageIDs(*remoteFile) {
				rc, downloadErr := e.storage.DownloadFile(ctx, groupID, topicID, messageID, remoteFile.Meta.Path, remoteFile.Size)
				if downloadErr != nil {
					closeReaders(closers)
					return fmt.Errorf("error downloading file %s: %w", item.Path, downloadErr)
				}
				readers = append(readers, rc)
				closers = append(closers, rc)
			}
			reader = io.MultiReader(readers...)
		}
		if err := e.fs.WriteFile(fullPath, reader); err != nil {
			closeReaders(closers)
			return fmt.Errorf("error writing file %s: %w", item.Path, err)
		}
		closeReaders(closers)

		// Restore original modification time
		if remoteFile.Meta.ModTime > 0 {
			if err := e.fs.SetModTime(fullPath, remoteFile.Meta.ModTime); err != nil {
				log.Printf("[!] Warning: failed to set modification time for %s: %v", item.Path, err)
			}
		}
		return nil
	}

	return retry.WithRetry(ctx, "Pull: "+item.Path, operation, 5, 1*time.Second)
}

func closeReaders(readers []io.Closer) {
	for _, reader := range readers {
		_ = reader.Close()
	}
}

func (e *executor) openChunkReaders(ctx context.Context, file domain.RemoteFile, groupID, topicID int64) (io.Reader, []io.Closer, error) {
	readers := make([]io.Reader, 0, len(file.ChunkIDs))
	closers := make([]io.Closer, 0, len(file.ChunkIDs))
	for _, messageID := range file.ChunkIDs {
		reader, err := e.storage.DownloadFile(ctx, groupID, topicID, messageID, file.Meta.Path, file.Size)
		if err != nil {
			closeReaders(closers)
			return nil, nil, err
		}
		readers = append(readers, reader)
		closers = append(closers, reader)
	}
	return io.MultiReader(readers...), closers, nil
}

func remoteMessageIDs(file domain.RemoteFile) []int {
	if len(file.ChunkIDs) > 0 {
		return file.ChunkIDs
	}
	return []int{file.MessageID}
}

func (e *executor) deleteRemote(ctx context.Context, item domain.SyncItem, groupID, topicID int64) error {
	if item.RemoteFile == nil {
		return fmt.Errorf("remote file is nil for delete: %s", item.Path)
	}
	log.Printf("[-] Deleting remote file: %s", item.Path)
	for _, messageID := range remoteMessageIDs(*item.RemoteFile) {
		if err := e.storage.DeleteFile(ctx, groupID, topicID, messageID); err != nil {
			return err
		}
	}
	remotePath := item.RemoteFile.Meta.Path
	e.mu.Lock()
	e.result.Deleted = append(e.result.Deleted, domain.DeletedFile{Path: remotePath})
	e.mu.Unlock()
	return nil
}

func (e *executor) deleteLocal(item domain.SyncItem, rootDir string) error {
	log.Printf("[-] Deleting local file: %s", item.Path)
	fullPath := filepath.Join(rootDir, item.Path)
	return e.fs.DeleteFile(fullPath)
}
