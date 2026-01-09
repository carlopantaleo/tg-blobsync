package usecase

import (
	"context"
	"log"
	"tg-blobsync/internal/domain"
)

type Synchronizer struct {
	fs      domain.FileSystem
	storage domain.BlobStorage
	workers int
	ui      domain.UserInterface
	skipMD5 bool
	subDir  string
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
	differ := NewDiffer(s.skipMD5)
	plan := differ.DiffPush(localFiles, remoteFiles)

	log.Printf("Sync Summary (Push):")
	log.Printf("  Local files:  %d", len(localFiles))
	log.Printf("  Remote files: %d", len(remoteFiles))
	log.Printf("  To Upload:    %d", plan.Summary.ToUpload)
	log.Printf("  To Update:    %d", plan.Summary.ToUpdate)
	log.Printf("  To Delete:    %d", plan.Summary.ToDelete)

	// 3. Execute
	executor := NewExecutor(s.fs, s.storage, s.workers, s.ui)
	return executor.Execute(ctx, plan, rootDir, groupID, topicID)
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
	differ := NewDiffer(s.skipMD5)
	plan := differ.DiffPull(localFiles, remoteFiles)

	log.Printf("Sync Summary (Pull):")
	log.Printf("  Local files:  %d", len(localFiles))
	log.Printf("  Remote files: %d", len(remoteFiles))
	log.Printf("  To Download:  %d", plan.Summary.ToDownload)
	log.Printf("  To Update:    %d", plan.Summary.ToUpdate)
	log.Printf("  To Delete:    %d", plan.Summary.ToDelete)

	// 3. Execute
	executor := NewExecutor(s.fs, s.storage, s.workers, s.ui)
	return executor.Execute(ctx, plan, rootDir, groupID, topicID)
}
