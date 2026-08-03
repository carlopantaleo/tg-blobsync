package usecase

import (
	"context"
	"testing"
	"tg-blobsync/internal/domain"
)

// chunkedBrowseUI is a BrowseUI mock that returns a DownloadRequest for the
// first file, capturing the files passed to BrowseFiles for assertions.
type chunkedBrowseUI struct {
	files   []domain.RemoteFile
	request *domain.DownloadRequest
}

func (u *chunkedBrowseUI) BrowseFiles(files []domain.RemoteFile) (interface{}, error) {
	u.files = files
	if u.request != nil {
		return u.request, nil
	}
	return nil, nil
}

func (u *chunkedBrowseUI) ConfirmCreateIndex(message string) (bool, error) {
	return false, nil
}

// TestBrowser_ListsChunkedEntryOnce verifies that browse lists an indexed
// chunked entry as a single file and preserves ordered chunk IDs.
func TestBrowser_ListsChunkedEntryOnce(t *testing.T) {
	mockStorage := NewMockBlobStorage()
	mockStorage.Indexes[100] = map[int64]*domain.FileIndex{200: {Entries: []domain.FileIndexEntry{
		{
			Path:      "large.bin",
			Size:      300,
			MessageID: 10,
			Flags:     domain.ChunkFlag,
			ChunkIDs:  []int{10, 11, 12},
		},
	}}}
	mockStorage.IndexIDs[100] = map[int64]int{200: 500}
	mockUI := &chunkedBrowseUI{}

	if err := NewBrowser(mockStorage, mockUI).ListAndBrowse(context.Background(), 100, 200); err != nil {
		t.Fatalf("ListAndBrowse failed: %v", err)
	}

	if len(mockUI.files) != 1 {
		t.Fatalf("expected 1 file in browse list, got %d", len(mockUI.files))
	}
	f := mockUI.files[0]
	if len(f.ChunkIDs) != 3 || f.ChunkIDs[0] != 10 || f.ChunkIDs[1] != 11 || f.ChunkIDs[2] != 12 {
		t.Errorf("chunk IDs = %#v, want [10, 11, 12]", f.ChunkIDs)
	}
	if f.Size != 300 {
		t.Errorf("size = %d, want 300", f.Size)
	}
}

// TestBrowser_DownloadRequestPreservesChunkedFile verifies that the download
// request issued by browse carries the chunked remote file with chunk IDs.
func TestBrowser_DownloadRequestPreservesChunkedFile(t *testing.T) {
	mockStorage := NewMockBlobStorage()
	mockStorage.Indexes[100] = map[int64]*domain.FileIndex{200: {Entries: []domain.FileIndexEntry{
		{
			Path:      "large.bin",
			Size:      300,
			MessageID: 10,
			Flags:     domain.ChunkFlag,
			ChunkIDs:  []int{10, 11, 12},
		},
	}}}
	mockStorage.IndexIDs[100] = map[int64]int{200: 500}

	expectedReq := &domain.DownloadRequest{File: domain.RemoteFile{
		Meta:      domain.FileMeta{Path: "large.bin", Flags: domain.ChunkFlag},
		Size:      300,
		MessageID: 10,
		ChunkIDs:  []int{10, 11, 12},
	}}
	mockUI := &chunkedBrowseUI{request: expectedReq}

	err := NewBrowser(mockStorage, mockUI).ListAndBrowse(context.Background(), 100, 200)
	if err == nil {
		t.Fatal("expected NavigationError for download request")
	}
	navErr, ok := err.(*domain.NavigationError)
	if !ok || navErr.Type != "download" {
		t.Fatalf("expected download NavigationError, got %v", err)
	}
	req, ok := navErr.Data.(*domain.DownloadRequest)
	if !ok {
		t.Fatalf("expected DownloadRequest in nav error, got %T", navErr.Data)
	}
	if len(req.File.ChunkIDs) != 3 {
		t.Errorf("download request chunk IDs = %#v, want 3 chunks", req.File.ChunkIDs)
	}
}
