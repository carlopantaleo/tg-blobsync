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
	RemoteIndexMessageID() int
}

type scanner struct {
	fs             domain.FileSystem
	storage        domain.BlobStorage
	subDir         string
	skipMD5        bool
	indexMessageID int
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
		// The path is already relative to rootDir.
		// We use this relative path as the key for synchronization.
		result[path] = f
	}
	return result, nil
}

func (s *scanner) ScanRemote(ctx context.Context, groupID, topicID int64) (map[string]domain.RemoteFile, error) {
	index, indexMessageID, indexed, err := s.storage.GetIndex(ctx, groupID, topicID)
	if err != nil {
		return nil, fmt.Errorf("failed to read remote index: %w", err)
	}
	if indexed {
		s.indexMessageID = indexMessageID
		return s.remoteMap(index.RemoteFiles()), nil
	}

	s.indexMessageID, err = MigrateTopicIndex(ctx, s.storage, groupID, topicID)
	if err != nil {
		return nil, err
	}
	files, err := s.storage.ListFiles(ctx, groupID, topicID)
	if err != nil {
		return nil, fmt.Errorf("failed to list remote files after migration: %w", err)
	}
	return s.remoteMap(files), nil
}

func (s *scanner) RemoteIndexMessageID() int {
	return s.indexMessageID
}

func (s *scanner) remoteMap(files []domain.RemoteFile) map[string]domain.RemoteFile {
	result := make(map[string]domain.RemoteFile)
	for _, f := range files {
		path := filepath.ToSlash(f.Meta.Path)
		if s.subDir != "" {
			if !strings.HasPrefix(path, s.subDir+"/") && path != s.subDir {
				continue
			}
			if path == s.subDir {
				path = filepath.Base(path)
			} else {
				path = strings.TrimPrefix(path, s.subDir+"/")
			}
		}
		if _, exists := result[path]; !exists {
			result[path] = f
		}
	}
	return result
}
