package usecase

import (
	"context"
	"fmt"
	"log"
	"tg-blobsync/internal/domain"
)

type Synchronizer struct {
	fs       domain.FileSystem
	storage  domain.BlobStorage
	workers  int
	ui       domain.UserInterface
	skipMD5  bool
	noDelete bool
	subDir   string
}

func NewSynchronizer(
	fs domain.FileSystem,
	storage domain.BlobStorage,
	workers int,
	ui domain.UserInterface,
	skipMD5 bool,
) *Synchronizer {
	return &Synchronizer{
		fs:      fs,
		storage: storage,
		workers: workers,
		ui:      ui,
		skipMD5: skipMD5,
	}
}

func (s *Synchronizer) SetSubDir(subDir string) {
	s.subDir = subDir
}

func (s *Synchronizer) SetNoDelete(noDelete bool) {
	s.noDelete = noDelete
}

func (s *Synchronizer) Push(ctx context.Context, rootDir string, groupID, topicID int64) error {
	log.Println("Starting Push synchronization...")

	// 1. Scan
	scanner := NewScanner(s.fs, s.storage, s.subDir, s.skipMD5)

	localFiles, err := scanner.ScanLocal(rootDir)
	if err != nil {
		return err
	}

	remoteFiles, err := scanner.ScanRemote(ctx, groupID, topicID)
	if err != nil {
		return err
	}

	// 2. Diff
	differ := NewDiffer(s.skipMD5, s.noDelete)
	plan := differ.DiffPush(localFiles, remoteFiles)

	log.Printf("Sync Summary (Push):")
	log.Printf("  Local files:  %d", len(localFiles))
	log.Printf("  Remote files: %d", len(remoteFiles))
	log.Printf("  To Upload:    %d", plan.Summary.ToUpload)
	log.Printf("  To Update:    %d", plan.Summary.ToUpdate)
	log.Printf("  To Delete:    %d", plan.Summary.ToDelete)
	log.Printf("  Total files:  %d (%s)", plan.Summary.Total, formatSize(plan.Summary.TotalSize))

	// 3. Execute
	executor := NewExecutor(s.fs, s.storage, s.workers, s.ui)
	executor.SetSubDir(s.subDir)
	result, err := executor.Execute(ctx, plan, rootDir, groupID, topicID)
	if err != nil {
		return err
	}
	if plan.Summary.Total > 0 {
		return s.updateIndex(ctx, groupID, topicID, scanner, result)
	}
	return nil
}

func (s *Synchronizer) Pull(ctx context.Context, rootDir string, groupID, topicID int64) error {
	log.Println("Starting Pull synchronization...")

	// 1. Scan
	scanner := NewScanner(s.fs, s.storage, s.subDir, s.skipMD5)

	// Note: ScanRemote is called first in original Pull, but order doesn't strictly matter
	// unless we want to fail fast on network.
	remoteFiles, err := scanner.ScanRemote(ctx, groupID, topicID)
	if err != nil {
		return err
	}

	localFiles, err := scanner.ScanLocal(rootDir)
	if err != nil {
		return err
	}

	// 2. Diff
	differ := NewDiffer(s.skipMD5, s.noDelete)
	plan := differ.DiffPull(localFiles, remoteFiles)

	log.Printf("Sync Summary (Pull):")
	log.Printf("  Local files:  %d", len(localFiles))
	log.Printf("  Remote files: %d", len(remoteFiles))
	log.Printf("  To Download:  %d", plan.Summary.ToDownload)
	log.Printf("  To Update:    %d", plan.Summary.ToUpdate)
	log.Printf("  To Delete:    %d", plan.Summary.ToDelete)
	log.Printf("  Total files:  %d (%s)", plan.Summary.Total, formatSize(plan.Summary.TotalSize))

	// 3. Execute
	executor := NewExecutor(s.fs, s.storage, s.workers, s.ui)
	executor.SetSubDir(s.subDir)
	result, err := executor.Execute(ctx, plan, rootDir, groupID, topicID)
	if err != nil {
		return err
	}
	if plan.Summary.Total > 0 {
		return s.updateIndex(ctx, groupID, topicID, scanner, result)
	}
	return nil
}

// updateIndex updates the topic index after synchronization. For indexed
// topics, it applies the operation delta to the retained index snapshot and
// deletes only the known previous index message. For legacy topics, it falls
// back to a full-topic rebuild.
func (s *Synchronizer) updateIndex(ctx context.Context, groupID, topicID int64, sc FileScanner, result domain.SyncResult) error {
	if sc.IsIndexed() && sc.RetainedIndex() != nil {
		return s.updateIndexByDelta(ctx, groupID, topicID, sc.RemoteIndexMessageID(), sc.RetainedIndex(), result)
	}
	return s.rebuildIndex(ctx, groupID, topicID, sc.RemoteIndexMessageID())
}

// updateIndexByDelta applies the synchronization delta to the retained index
// snapshot, deletes the previous known index message, and uploads a fresh
// index. This avoids full-topic reads when the topic was already indexed.
func (s *Synchronizer) updateIndexByDelta(ctx context.Context, groupID, topicID int64, oldIndexID int, retained *domain.FileIndex, result domain.SyncResult) error {
	deletedPaths := make(map[string]bool)
	for _, d := range result.Deleted {
		deletedPaths[d.Path] = true
	}
	uploadedByPath := make(map[string]domain.UploadedFile)
	for _, u := range result.Uploaded {
		uploadedByPath[u.Path] = u
	}

	// Build the new index from the retained snapshot, applying the delta
	var entries []domain.FileIndexEntry
	for _, entry := range retained.Entries {
		if deletedPaths[entry.Path] {
			continue
		}
		if uploaded, ok := uploadedByPath[entry.Path]; ok {
			// Replace with the uploaded version
			entries = append(entries, domain.FileIndexEntry{
				Path:      uploaded.Path,
				Checksum:  uploaded.Checksum,
				ModTime:   uploaded.ModTime,
				Flags:     uploaded.Flags,
				Size:      uploaded.Size,
				MessageID: uploaded.MessageID,
				ChunkIDs:  uploaded.ChunkIDs,
			})
			delete(uploadedByPath, entry.Path)
			continue
		}
		entries = append(entries, entry)
	}
	// Add newly uploaded files that weren't replacements
	for _, uploaded := range uploadedByPath {
		entries = append(entries, domain.FileIndexEntry{
			Path:      uploaded.Path,
			Checksum:  uploaded.Checksum,
			ModTime:   uploaded.ModTime,
			Flags:     uploaded.Flags,
			Size:      uploaded.Size,
			MessageID: uploaded.MessageID,
			ChunkIDs:  uploaded.ChunkIDs,
		})
	}

	newIndex := domain.FileIndex{Entries: entries}

	// Delete the previous known index message
	if oldIndexID != 0 {
		if err := s.storage.DeleteFile(ctx, groupID, topicID, oldIndexID); err != nil {
			return fmt.Errorf("failed to delete old index %d: %w", oldIndexID, err)
		}
	}

	if _, err := s.storage.UploadIndex(ctx, groupID, topicID, newIndex); err != nil {
		return fmt.Errorf("failed to upload updated index: %w", err)
	}
	return nil
}

func (s *Synchronizer) rebuildIndex(ctx context.Context, groupID, topicID int64, currentIndexID int) error {
	files, err := s.storage.ListFiles(ctx, groupID, topicID)
	if err != nil {
		return fmt.Errorf("failed to list files for index rebuild: %w", err)
	}
	indexIDs, err := s.storage.ListIndexMessageIDs(ctx, groupID, topicID)
	if err != nil {
		return fmt.Errorf("failed to list indexes for rebuild: %w", err)
	}
	if currentIndexID != 0 {
		indexIDs = append(indexIDs, currentIndexID)
	}
	seen := make(map[int]struct{})
	for _, messageID := range indexIDs {
		if _, exists := seen[messageID]; exists {
			continue
		}
		seen[messageID] = struct{}{}
		if err := s.storage.DeleteFile(ctx, groupID, topicID, messageID); err != nil {
			return fmt.Errorf("failed to delete old index %d: %w", messageID, err)
		}
	}
	if _, err := s.storage.UploadIndex(ctx, groupID, topicID, domain.NewFileIndex(files)); err != nil {
		return fmt.Errorf("failed to upload rebuilt index: %w", err)
	}
	return nil
}

func formatSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
