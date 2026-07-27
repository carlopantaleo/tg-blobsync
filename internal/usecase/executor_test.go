package usecase

import (
	"context"
	"path/filepath"
	"testing"
	"tg-blobsync/internal/domain"
)

func TestExecutor_Execute(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockStorage := NewMockBlobStorage()
	mockUI := &MockUserInterface{Confirmed: true}

	executor := NewExecutor(mockFS, mockStorage, 1, mockUI)

	ctx := context.Background()
	groupID := int64(100)
	topicID := int64(200)
	rootDir := filepath.Join("tmp", "test")

	// 1. Test Upload
	localFile := domain.LocalFile{Path: "test.txt", AbsPath: filepath.Join(rootDir, "test.txt")}
	planUpload := domain.SyncPlan{
		Items: []domain.SyncItem{
			{
				Path:      "test.txt",
				Action:    domain.ActionUpload,
				LocalFile: &localFile,
			},
		},
		Summary: domain.SyncSummary{Total: 1, ToUpload: 1},
	}

	if err := executor.Execute(ctx, planUpload, rootDir, groupID, topicID); err != nil {
		t.Errorf("Execute(Upload) failed: %v", err)
	}

	// 2. Test Download
	remoteFile := domain.RemoteFile{Meta: domain.FileMeta{Path: "remote.txt"}, Size: 100, MessageID: 1}
	planDownload := domain.SyncPlan{
		Items: []domain.SyncItem{
			{
				Path:       "remote.txt",
				Action:     domain.ActionDownload,
				RemoteFile: &remoteFile,
			},
		},
		Summary: domain.SyncSummary{Total: 1, ToDownload: 1},
	}

	if err := executor.Execute(ctx, planDownload, rootDir, groupID, topicID); err != nil {
		t.Errorf("Execute(Download) failed: %v", err)
	}

	// Check if file was written to mockFS
	// Use filepath.Join to match what Executor uses
	expectedPath := filepath.Join(rootDir, "remote.txt")
	if _, ok := mockFS.Data[expectedPath]; !ok {
		t.Errorf("Expected file to be written to FS at %s", expectedPath)
	}

	// 3. Test Delete Local
	localDel := domain.LocalFile{Path: "del.txt"}
	delPath := filepath.Join(rootDir, "del.txt")
	mockFS.Data[delPath] = []byte("content")

	planDelLocal := domain.SyncPlan{
		Items: []domain.SyncItem{
			{
				Path:      "del.txt",
				Action:    domain.ActionDeleteLocal,
				LocalFile: &localDel,
			},
		},
		Summary: domain.SyncSummary{Total: 1, ToDelete: 1},
	}

	if err := executor.Execute(ctx, planDelLocal, rootDir, groupID, topicID); err != nil {
		t.Errorf("Execute(DeleteLocal) failed: %v", err)
	}
	if _, ok := mockFS.Data[delPath]; ok {
		t.Error("File should have been deleted from mockFS")
	}

	// 4. Test User Cancel
	mockUI.Confirmed = false
	if err := executor.Execute(ctx, planUpload, rootDir, groupID, topicID); err != nil {
		t.Errorf("Execute(Cancel) failed: %v", err)
	}
	// Should log "cancelled" and return nil

	// 5. Test Upload with SubDir
	mockUI.Confirmed = true
	executor.SetSubDir("remote-subdir")
	if err := executor.Execute(ctx, planUpload, rootDir, groupID, topicID); err != nil {
		t.Errorf("Execute(Upload with SubDir) failed: %v", err)
	}

	// Verify that the file was uploaded with the correct remote path
	remoteFiles := mockStorage.Files[groupID][topicID]
	found := false
	expectedRemotePath := "remote-subdir/test.txt"
	for _, f := range remoteFiles {
		if f.Meta.Path == expectedRemotePath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected file to be uploaded to %s, but not found in mock storage", expectedRemotePath)
	}

	// 6. Test Download with SubDir
	// remotePath is what's on Telegram, path is what the syncer/differ uses (relative)
	remoteFileSubDir := domain.RemoteFile{
		Meta:      domain.FileMeta{Path: "remote-subdir/file_in_sub.txt"},
		Size:      100,
		MessageID: 2,
	}
	planDownloadSubDir := domain.SyncPlan{
		Items: []domain.SyncItem{
			{
				Path:       "file_in_sub.txt", // relative path after mapping
				Action:     domain.ActionDownload,
				RemoteFile: &remoteFileSubDir,
			},
		},
		Summary: domain.SyncSummary{Total: 1, ToDownload: 1},
	}

	if err := executor.Execute(ctx, planDownloadSubDir, rootDir, groupID, topicID); err != nil {
		t.Errorf("Execute(Download with SubDir) failed: %v", err)
	}

	expectedLocalPath := filepath.Join(rootDir, "file_in_sub.txt")
	if _, ok := mockFS.Data[expectedLocalPath]; !ok {
		t.Errorf("Expected file to be written locally at %s", expectedLocalPath)
	}
}

func TestExecutor_ChunkedDownloadAndDelete(t *testing.T) {
	storage := NewMockBlobStorage()
	fs := NewMockFileSystem()
	executor := NewExecutor(fs, storage, 1, &MockUserInterface{Confirmed: true})
	remote := domain.RemoteFile{Meta: domain.FileMeta{Path: "large.bin"}, ChunkIDs: []int{10, 11}, Size: 20}
	plan := domain.SyncPlan{Items: []domain.SyncItem{{Path: "large.bin", Action: domain.ActionDownload, RemoteFile: &remote}}, Summary: domain.SyncSummary{Total: 1}}
	if err := executor.Execute(context.Background(), plan, "root", 1, 2); err != nil {
		t.Fatalf("chunked download: %v", err)
	}
	if got := string(fs.Data[filepath.Join("root", "large.bin")]); got != "dummy contentdummy content" {
		t.Fatalf("downloaded data = %q", got)
	}

	deletePlan := domain.SyncPlan{Items: []domain.SyncItem{{Path: "large.bin", Action: domain.ActionDeleteRemote, RemoteFile: &remote}}, Summary: domain.SyncSummary{Total: 1}}
	if err := executor.Execute(context.Background(), deletePlan, "root", 1, 2); err != nil {
		t.Fatalf("chunked delete: %v", err)
	}
	if len(storage.DeletedIDs) != 2 || storage.DeletedIDs[0] != 10 || storage.DeletedIDs[1] != 11 {
		t.Fatalf("deleted IDs = %#v", storage.DeletedIDs)
	}
}

func TestExecutor_DeleteRemote(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockStorage := NewMockBlobStorage()
	mockUI := &MockUserInterface{Confirmed: true}

	executor := NewExecutor(mockFS, mockStorage, 1, mockUI)
	ctx := context.Background()
	groupID := int64(10)
	topicID := int64(20)
	plan := domain.SyncPlan{
		Items: []domain.SyncItem{{
			Path:       "obsolete.txt",
			Action:     domain.ActionDeleteRemote,
			RemoteFile: &domain.RemoteFile{MessageID: 99},
		}},
		Summary: domain.SyncSummary{Total: 1, ToDelete: 1},
	}

	if err := executor.Execute(ctx, plan, "root", groupID, topicID); err != nil {
		t.Fatalf("Execute(DeleteRemote) error: %v", err)
	}
	if mockStorage.LastDeleted.MessageID != 99 || mockStorage.LastDeleted.GroupID != groupID || mockStorage.LastDeleted.TopicID != topicID {
		t.Fatalf("DeleteFile not called with expected ids: %+v", mockStorage.LastDeleted)
	}
}
