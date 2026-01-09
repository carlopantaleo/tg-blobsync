package usecase

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"tg-blobsync/internal/domain"
	"tg-blobsync/internal/pkg/retry"
	"time"

	"golang.org/x/sync/errgroup"
)

type SyncExecutor interface {
	Execute(ctx context.Context, plan domain.SyncPlan, rootDir string, groupID, topicID int64) error
}

type executor struct {
	fs      domain.FileSystem
	storage domain.BlobStorage
	workers int
	ui      domain.UserInterface
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

func (e *executor) Execute(ctx context.Context, plan domain.SyncPlan, rootDir string, groupID, topicID int64) error {
	if plan.Summary.Total == 0 {
		log.Println("Everything is up to date.")
		return nil
	}

	// User Confirmation
	if e.ui != nil {
		confirmed, err := e.ui.ConfirmSync(plan)
		if err != nil {
			return err
		}
		if !confirmed {
			log.Println("Sync cancelled by user.")
			return nil
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
		return err
	}

	if e.ui != nil {
		e.ui.Wait()
	}

	// Execute Deletions
	for _, item := range deleteTasks {
		if err := e.processItem(ctx, item, rootDir, groupID, topicID); err != nil {
			log.Printf("Error processing delete for %s: %v", item.Path, err)
		}
	}

	return nil
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

	err := e.storage.UploadFile(ctx, groupID, topicID, *item.LocalFile)
	if err != nil {
		return fmt.Errorf("error uploading file %s: %w", item.Path, err)
	}

	// If it was an update (RemoteFile exists), delete the old version on Telegram
	if item.RemoteFile != nil {
		log.Printf("[*] Deleting old version of: %s", item.Path)
		err := e.storage.DeleteFile(ctx, groupID, topicID, item.RemoteFile.MessageID)
		if err != nil {
			log.Printf("Warning: failed to delete old version of %s: %v", item.Path, err)
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
		if remoteFile.Meta.Flags == "EMPTY_FILE" {
			log.Printf("[*] Restoring empty file: %s", item.Path)
			if err := e.fs.WriteFile(fullPath, strings.NewReader("")); err != nil {
				return fmt.Errorf("error creating empty file %s: %w", item.Path, err)
			}
			if err := e.fs.SetModTime(fullPath, remoteFile.Meta.ModTime); err != nil {
				log.Printf("Warning: failed to set modification time for %s: %v", item.Path, err)
			}
			return nil
		}

		rc, err := e.storage.DownloadFile(ctx, groupID, topicID, remoteFile.MessageID, remoteFile.Meta.Path, remoteFile.Size)
		if err != nil {
			return fmt.Errorf("error downloading file %s: %w", item.Path, err)
		}
		defer rc.Close()

		if err := e.fs.WriteFile(fullPath, rc); err != nil {
			return fmt.Errorf("error writing file %s: %w", item.Path, err)
		}

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

func (e *executor) deleteRemote(ctx context.Context, item domain.SyncItem, groupID, topicID int64) error {
	if item.RemoteFile == nil {
		return fmt.Errorf("remote file is nil for delete: %s", item.Path)
	}
	log.Printf("[-] Deleting remote file: %s", item.Path)
	return e.storage.DeleteFile(ctx, groupID, topicID, item.RemoteFile.MessageID)
}

func (e *executor) deleteLocal(item domain.SyncItem, rootDir string) error {
	log.Printf("[-] Deleting local file: %s", item.Path)
	fullPath := filepath.Join(rootDir, item.Path)
	return e.fs.DeleteFile(fullPath)
}
