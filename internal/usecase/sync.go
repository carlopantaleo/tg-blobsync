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
	fs      domain.FileSystem
	storage domain.BlobStorage
	workers int
}

func NewSynchronizer(fs domain.FileSystem, storage domain.BlobStorage, workers int) *Synchronizer {
	if workers <= 0 {
		workers = 1
	}
	return &Synchronizer{
		fs:      fs,
		storage: storage,
		workers: workers,
	}
}

// Push synchronizes local folder to remote topic.
func (s *Synchronizer) Push(ctx context.Context, rootDir string, groupID, topicID int64) error {
	log.Println("Starting Push synchronization...")

	// 1. Analyze Local
	localFiles, err := s.fs.ListFiles(rootDir)
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

	// 3. Upload (Upsert)
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(s.workers)

	for path, localFile := range localMap {
		path := path
		localFile := localFile
		remoteFile, exists := remoteMap[path]

		shouldUpload := false
		if !exists {
			log.Printf("[+] Uploading new file: %s", path)
			shouldUpload = true
		} else if remoteFile.Meta.Checksum != localFile.Checksum {
			log.Printf("[*] Updating modified file: %s", path)
			shouldUpload = true
		}

		if shouldUpload {
			g.Go(func() error {
				f, err := s.fs.ReadFile(localFile.AbsPath)
				if err != nil {
					return fmt.Errorf("error reading local file %s: %w", path, err)
				}
				defer f.Close()

				err = s.storage.UploadFile(gCtx, groupID, topicID, localFile, f)
				if err != nil {
					return fmt.Errorf("error uploading file %s: %w", path, err)
				}
				return nil
			})
		}
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// 4. Pruning (Delete from remote if not in local)
	for path, remoteFile := range remoteMap {
		if _, exists := localMap[path]; !exists {
			log.Printf("[-] Deleting remote file: %s", path)
			err := s.storage.DeleteFile(ctx, groupID, topicID, remoteFile.MessageID)
			if err != nil {
				log.Printf("Error deleting remote file %s: %v", path, err)
			}
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
	localFiles, err := s.fs.ListFiles(rootDir)
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
		localFiles, err = s.fs.ListFiles(rootDir)
		if err != nil {
			return fmt.Errorf("failed to list local files: %w", err)
		}
	}
	localMap := make(map[string]domain.LocalFile)
	for _, f := range localFiles {
		localMap[f.Path] = f
	}

	// 3. Download
	dg, dgCtx := errgroup.WithContext(ctx)
	dg.SetLimit(s.workers)

	for path, remoteFile := range remoteMap {
		path := path
		remoteFile := remoteFile
		localFile, exists := localMap[path]

		shouldDownload := false
		if !exists {
			log.Printf("[+] Downloading new file: %s", path)
			shouldDownload = true
		} else if localFile.Checksum != remoteFile.Meta.Checksum {
			log.Printf("[*] Updating modified file: %s", path)
			shouldDownload = true
		}

		if shouldDownload {
			dg.Go(func() error {
				rc, err := s.storage.DownloadFile(dgCtx, groupID, topicID, remoteFile.MessageID)
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
	}

	if err := dg.Wait(); err != nil {
		return err
	}

	// 4. Pruning (Delete local if not in remote)
	for path := range localMap {
		if _, exists := remoteMap[path]; !exists {
			log.Printf("[-] Deleting local file: %s", path)
			fullPath := filepath.Join(rootDir, path)
			err := s.fs.DeleteFile(fullPath)
			if err != nil {
				log.Printf("Error deleting local file %s: %v", path, err)
			}
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
