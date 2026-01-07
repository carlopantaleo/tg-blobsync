package filesystem

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"tg-blobsync/internal/domain"
)

type LocalFileSystem struct{}

func NewLocalFileSystem() *LocalFileSystem {
	return &LocalFileSystem{}
}

// ListFiles recursively scans the root directory and returns a list of files with their metadata.
func (l *LocalFileSystem) ListFiles(root string) ([]domain.LocalFile, error) {
	var files []domain.LocalFile

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Calculate relative path
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
        
        // Normalize path separators to forward slashes for consistency across platforms
        relPath = filepath.ToSlash(relPath)

		// Calculate MD5
		checksum, err := l.calculateMD5(path)
		if err != nil {
			return fmt.Errorf("failed to calculate md5 for %s: %w", path, err)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		files = append(files, domain.LocalFile{
			Path:     relPath,
			Checksum: checksum,
			Size:     info.Size(),
			AbsPath:  path,
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

func (l *LocalFileSystem) calculateMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func (l *LocalFileSystem) ReadFile(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (l *LocalFileSystem) WriteFile(path string, data io.Reader) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, data)
	return err
}

func (l *LocalFileSystem) DeleteFile(path string) error {
	return os.Remove(path)
}

func (l *LocalFileSystem) EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}
