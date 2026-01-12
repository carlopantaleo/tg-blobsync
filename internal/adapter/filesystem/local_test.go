package filesystem

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalFileSystem(t *testing.T) {
	lfs := NewLocalFileSystem()
	tmpDir := t.TempDir()

	// 1. Test EnsureDir & WriteFile
	testPath := filepath.Join(tmpDir, "subdir", "test.txt")
	content := []byte("hello world")

	// EnsureDir is called implicitly by WriteFile in current implementation?
	// Looking at code: WriteFile calls os.MkdirAll(dir).
	// Let's test WriteFile first.
	if err := lfs.WriteFile(testPath, bytes.NewReader(content)); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		t.Fatalf("File was not created at %s", testPath)
	}

	// 2. Test ReadFile
	rc, err := lfs.ReadFile(testPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	readContent, err := io.ReadAll(rc)
	rc.Close() // Close explicitly before deletion tests
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if !bytes.Equal(readContent, content) {
		t.Errorf("Content mismatch: got %s, want %s", readContent, content)
	}

	// 3. Test ListFiles
	// Add another file
	testPath2 := filepath.Join(tmpDir, "root.txt")
	content2 := []byte("root file")
	if err := lfs.WriteFile(testPath2, bytes.NewReader(content2)); err != nil {
		t.Fatalf("WriteFile 2 failed: %v", err)
	}

	// List files with MD5
	files, err := lfs.ListFiles(tmpDir, false)
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}

	// Verify MD5 of first file
	hasher := md5.New()
	hasher.Write(content)
	expectedMD5 := hex.EncodeToString(hasher.Sum(nil))

	found := false
	for _, f := range files {
		// Normalize separators for comparison if needed, but ListFiles should return relative paths
		// checking if one of the files matches test.txt (relative path should be subdir/test.txt or subdir\test.txt)
		// We can check equality by content/checksum mainly or loosely check path suffix
		if f.Size == int64(len(content)) && f.Checksum == expectedMD5 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Could not find test.txt with correct metadata in ListFiles output")
	}

	// List files without MD5
	filesNoMD5, err := lfs.ListFiles(tmpDir, true)
	if err != nil {
		t.Fatalf("ListFiles(skipMD5=true) failed: %v", err)
	}
	if len(filesNoMD5) != 2 {
		t.Errorf("Expected 2 files (skipMD5), got %d", len(filesNoMD5))
	}
	if filesNoMD5[0].Checksum != "" {
		t.Error("Expected empty checksum when skipMD5 is true")
	}

	// 4. Test SetModTime
	newTime := time.Now().Add(-1 * time.Hour).Unix()
	if err := lfs.SetModTime(testPath, newTime); err != nil {
		t.Fatalf("SetModTime failed: %v", err)
	}

	info, err := os.Stat(testPath)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	// precision might vary, compare with 1s tolerance
	if absDiff(info.ModTime().Unix(), newTime) > 1 {
		t.Errorf("ModTime mismatch: got %d, want %d", info.ModTime().Unix(), newTime)
	}

	// Test SetModTime with 0 (should do nothing)
	if err := lfs.SetModTime(testPath, 0); err != nil {
		t.Errorf("SetModTime(0) failed: %v", err)
	}

	// 5. Test DeleteFile
	if err := lfs.DeleteFile(testPath); err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}
	if _, err := os.Stat(testPath); !os.IsNotExist(err) {
		t.Error("File should be deleted")
	}

	// 6. Test EnsureDir explicitly
	dirPath := filepath.Join(tmpDir, "explicit_dir")
	if err := lfs.EnsureDir(dirPath); err != nil {
		t.Fatalf("EnsureDir failed: %v", err)
	}
	if info, err := os.Stat(dirPath); err != nil || !info.IsDir() {
		t.Error("Directory not created correctly")
	}
}

func TestLocalFileSystem_Errors(t *testing.T) {
	lfs := NewLocalFileSystem()
	tmpDir := t.TempDir()

	// 1. ListFiles on non-existent directory
	_, err := lfs.ListFiles(filepath.Join(tmpDir, "nonexistent"), false)
	if err == nil {
		t.Error("Expected error from ListFiles on non-existent dir")
	}

	// 2. ReadFile on non-existent file
	_, err = lfs.ReadFile(filepath.Join(tmpDir, "nonexistent.txt"))
	if err == nil {
		t.Error("Expected error from ReadFile on non-existent file")
	}

	// 3. WriteFile to invalid path (e.g. directory as file)
	dirPath := filepath.Join(tmpDir, "somedir")
	os.Mkdir(dirPath, 0755)
	err = lfs.WriteFile(dirPath, bytes.NewReader([]byte("test")))
	if err == nil {
		t.Error("Expected error from WriteFile to directory path")
	}
}

func absDiff(a, b int64) int64 {
	if a > b {
		return a - b
	}
	return b - a
}
