package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"tg-blobsync/internal/domain"

	"golang.org/x/sync/errgroup"
)

type Synchronizer struct {
	fs       domain.FileSystem
	storage  domain.BlobStorage
	workers  int
	reporter domain.ProgressReporter
	skipMD5  bool
}

func NewSynchronizer(fs domain.FileSystem, storage domain.BlobStorage, workers int, reporter domain.ProgressReporter, skipMD5 bool) *Synchronizer {
	if workers <= 0 {
		workers = 1
	}
	return &Synchronizer{
		fs:       fs,
		storage:  storage,
		workers:  workers,
		reporter: reporter,
		skipMD5:  skipMD5,
	}
}

// Push synchronizes local folder to remote topic.
func (s *Synchronizer) Push(ctx context.Context, rootDir string, groupID, topicID int64) error {
	log.Println("Starting Push synchronization...")

	// 1. Analyze Local
	localFiles, err := s.fs.ListFiles(rootDir, s.skipMD5)
	if err != nil {
		return fmt.Errorf("failed to list local files: %w", err)
	}
	localMap := make(map[string]domain.LocalFile)
	for _, f := range localFiles {
		localMap[f.Path] = f
	}

	// 2. Fetch Remote
	remoteFiles, err := s.storage.ListFiles(ctx, groupID, topicID)
	if err != nil {
		return fmt.Errorf("failed to list remote files: %w", err)
	}
	remoteMap := make(map[string]domain.RemoteFile)
	for _, f := range remoteFiles {
		remoteMap[f.Meta.Path] = f
	}

	// 3. Plan Operations
	var toUpload, toUpdate []string
	for path, localFile := range localMap {
		remoteFile, exists := remoteMap[path]
		if !exists {
			toUpload = append(toUpload, path)
		} else {
			shouldUpdate := false
			if s.skipMD5 {
				// Use ModTime and Size as comparison
				if remoteFile.Meta.ModTime != localFile.ModTime || remoteFile.Size != localFile.Size {
					shouldUpdate = true
				}
			} else {
				// Use Checksum
				if remoteFile.Meta.Checksum != localFile.Checksum {
					shouldUpdate = true
				}
			}

			if shouldUpdate {
				toUpdate = append(toUpdate, path)
			}
		}
	}

	var toDelete []string
	for path := range remoteMap {
		if _, exists := localMap[path]; !exists {
			toDelete = append(toDelete, path)
		}
	}

	log.Printf("----------------------------------------------------------")
	log.Printf("Sync Summary (Push):")
	log.Printf("  Local files:  %d", len(localMap))
	log.Printf("  Remote files: %d", len(remoteMap))
	log.Printf("  To Upload:    %d", len(toUpload))
	log.Printf("  To Update:    %d", len(toUpdate))
	log.Printf("  To Delete:    %d", len(toDelete))
	log.Printf("----------------------------------------------------------")

	if len(toUpload)+len(toUpdate)+len(toDelete) == 0 {
		log.Println("Everything is up to date.")
		return nil
	}

	// 4. Upload & Update (Upsert)
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(s.workers)

	// Combine upload and update tasks
	allTasks := append(toUpload, toUpdate...)

	// Use a map to track old remote files that need to be deleted after update
	updateMap := make(map[string]domain.RemoteFile)
	for _, path := range toUpdate {
		updateMap[path] = remoteMap[path]
	}

	for _, path := range allTasks {
		path := path
		localFile := localMap[path]

		g.Go(func() error {
			err = s.storage.UploadFile(gCtx, groupID, topicID, localFile)
			if err != nil {
				return fmt.Errorf("error uploading file %s: %w", path, err)
			}

			// If it was an update, delete the old version AFTER successful upload
			if oldFile, ok := updateMap[path]; ok {
				log.Printf("[*] Replacing old version of: %s", path)
				err := s.storage.DeleteFile(gCtx, groupID, topicID, oldFile.MessageID)
				if err != nil {
					log.Printf("Warning: failed to delete old version of %s: %v", path, err)
				}
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	if s.reporter != nil {
		s.reporter.Wait()
	}

	// 5. Pruning (Delete from remote if not in local)
	for _, path := range toDelete {
		remoteFile := remoteMap[path]
		log.Printf("[-] Deleting remote file: %s", path)
		err := s.storage.DeleteFile(ctx, groupID, topicID, remoteFile.MessageID)
		if err != nil {
			log.Printf("Error deleting remote file %s: %v", path, err)
		}
	}

	log.Println("Push synchronization completed.")
	return nil
}

// Pull synchronizes remote topic to local folder.
func (s *Synchronizer) Pull(ctx context.Context, rootDir string, groupID, topicID int64) error {
	log.Println("Starting Pull synchronization...")

	// 1. Fetch Remote
	remoteFiles, err := s.storage.ListFiles(ctx, groupID, topicID)
	if err != nil {
		return fmt.Errorf("failed to list remote files: %w", err)
	}
	remoteMap := make(map[string]domain.RemoteFile)
	for _, f := range remoteFiles {
		remoteMap[f.Meta.Path] = f
	}

	// 2. Analyze Local
	// We need to know what's local to prune or skip
	localFiles, err := s.fs.ListFiles(rootDir, s.skipMD5)
	if err != nil {
		// If directory doesn't exist, we might treat it as empty or create it.
		// For now assume ListFiles handles it or returns error.
		// If it's a "not found" error, maybe we just have 0 local files.
		// But usually ListFiles(rootDir) expects rootDir to exist.
		// Let's assume the caller ensures rootDir exists or we create it.
		// Actually, ListFiles calls filepath.WalkDir. If root doesn't exist it errors.
		// Let's ensure it exists.
		if err := s.fs.EnsureDir(rootDir); err != nil {
			return fmt.Errorf("failed to ensure root dir: %w", err)
		}
		// Try listing again
		localFiles, err = s.fs.ListFiles(rootDir, s.skipMD5)
		if err != nil {
			return fmt.Errorf("failed to list local files: %w", err)
		}
	}
	localMap := make(map[string]domain.LocalFile)
	for _, f := range localFiles {
		localMap[f.Path] = f
	}

	// 3. Plan Operations
	var toDownload, toUpdate []string
	for path, remoteFile := range remoteMap {
		localFile, exists := localMap[path]
		if !exists {
			toDownload = append(toDownload, path)
		} else {
			shouldUpdate := false
			if s.skipMD5 {
				if localFile.ModTime != remoteFile.Meta.ModTime || localFile.Size != remoteFile.Size {
					shouldUpdate = true
				}
			} else {
				if localFile.Checksum != remoteFile.Meta.Checksum {
					shouldUpdate = true
				}
			}

			if shouldUpdate {
				toUpdate = append(toUpdate, path)
			}
		}
	}

	var toDelete []string
	for path := range localMap {
		if _, exists := remoteMap[path]; !exists {
			toDelete = append(toDelete, path)
		}
	}

	log.Printf("----------------------------------------------------------")
	log.Printf("Sync Summary (Pull):")
	log.Printf("  Local files:  %d", len(localMap))
	log.Printf("  Remote files: %d", len(remoteMap))
	log.Printf("  To Download:  %d", len(toDownload))
	log.Printf("  To Update:    %d", len(toUpdate))
	log.Printf("  To Delete:    %d", len(toDelete))
	log.Printf("----------------------------------------------------------")

	if len(toDownload)+len(toUpdate)+len(toDelete) == 0 {
		log.Println("Everything is up to date.")
		return nil
	}

	// 4. Download
	dg, dgCtx := errgroup.WithContext(ctx)
	dg.SetLimit(s.workers)

	allTasks := append(toDownload, toUpdate...)

	for _, path := range allTasks {
		path := path
		remoteFile := remoteMap[path]

		dg.Go(func() error {
			rc, err := s.storage.DownloadFile(dgCtx, groupID, topicID, remoteFile.MessageID, remoteFile.Meta.Path, remoteFile.Size)
			if err != nil {
				return fmt.Errorf("error downloading file %s: %w", path, err)
			}
			defer rc.Close()

			fullPath := filepath.Join(rootDir, path)
			err = s.fs.WriteFile(fullPath, rc)
			if err != nil {
				return fmt.Errorf("error writing file %s: %w", path, err)
			}
			return nil
		})
	}

	if err := dg.Wait(); err != nil {
		return err
	}

	if s.reporter != nil {
		s.reporter.Wait()
	}

	// 5. Pruning (Delete local if not in remote)
	for _, path := range toDelete {
		log.Printf("[-] Deleting local file: %s", path)
		fullPath := filepath.Join(rootDir, path)
		err := s.fs.DeleteFile(fullPath)
		if err != nil {
			log.Printf("Error deleting local file %s: %v", path, err)
		}
	}

	log.Println("Pull synchronization completed.")
	return nil
}

// MetaToJSON helper
func MetaToJSON(meta domain.FileMeta) (string, error) {
	b, err := json.Marshal(meta)
	return string(b), err
}
