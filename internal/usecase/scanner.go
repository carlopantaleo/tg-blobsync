package usecase

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"tg-blobsync/internal/domain"
)

type FileScanner interface {
	ScanLocal(rootDir string) (map[string]domain.LocalFile, error)
	ScanRemote(ctx context.Context, groupID, topicID int64) (map[string]domain.RemoteFile, error)
}

type scanner struct {
	fs      domain.FileSystem
	storage domain.BlobStorage
	subDir  string
	skipMD5 bool
}

func NewScanner(fs domain.FileSystem, storage domain.BlobStorage, subDir string, skipMD5 bool) FileScanner {
	// Normalize subDir
	subDir = filepath.ToSlash(subDir)
	subDir = strings.Trim(subDir, "/")

	return &scanner{
		fs:      fs,
		storage: storage,
		subDir:  subDir,
		skipMD5: skipMD5,
	}
}

func (s *scanner) ScanLocal(rootDir string) (map[string]domain.LocalFile, error) {
	// Ensure rootDir exists
	if err := s.fs.EnsureDir(rootDir); err != nil {
		return nil, fmt.Errorf("failed to ensure root dir: %w", err)
	}

	files, err := s.fs.ListFiles(rootDir, s.skipMD5)
	if err != nil {
		return nil, fmt.Errorf("failed to list local files: %w", err)
	}

	result := make(map[string]domain.LocalFile)
	for _, f := range files {
		path := filepath.ToSlash(f.Path)
		if s.subDir != "" {
			if !strings.HasPrefix(path, s.subDir+"/") && path != s.subDir {
				continue
			}
		}
		result[path] = f
	}
	return result, nil
}

func (s *scanner) ScanRemote(ctx context.Context, groupID, topicID int64) (map[string]domain.RemoteFile, error) {
	files, err := s.storage.ListFiles(ctx, groupID, topicID)
	if err != nil {
		return nil, fmt.Errorf("failed to list remote files: %w", err)
	}

	result := make(map[string]domain.RemoteFile)
	for _, f := range files {
		path := filepath.ToSlash(f.Meta.Path)
		if s.subDir != "" {
			if !strings.HasPrefix(path, s.subDir+"/") && path != s.subDir {
				continue
			}
		}
		// Dedup: keep first (newest)
		if _, exists := result[path]; !exists {
			result[path] = f
		}
	}
	return result, nil
}
