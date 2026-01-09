package usecase

import (
	"context"
	"fmt"
	"tg-blobsync/internal/domain"
)

type FileBrowser interface {
	ListAndBrowse(ctx context.Context, groupID, topicID int64) error
}

type browser struct {
	storage domain.BlobStorage
	ui      BrowseUI
}

// BrowseUI defines the interface required by the browser use case for interaction
type BrowseUI interface {
	BrowseFiles(files []domain.RemoteFile) error
}

func NewBrowser(storage domain.BlobStorage, ui BrowseUI) FileBrowser {
	return &browser{
		storage: storage,
		ui:      ui,
	}
}

func (b *browser) ListAndBrowse(ctx context.Context, groupID, topicID int64) error {
	files, err := b.storage.ListFiles(ctx, groupID, topicID)
	if err != nil {
		return fmt.Errorf("failed to list files: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no files found in this topic")
	}

	return b.ui.BrowseFiles(files)
}
