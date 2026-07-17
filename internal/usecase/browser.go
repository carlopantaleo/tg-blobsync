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
	BrowseFiles(files []domain.RemoteFile) (interface{}, error)
	ConfirmCreateIndex(message string) (bool, error)
}

func NewBrowser(storage domain.BlobStorage, ui BrowseUI) FileBrowser {
	return &browser{
		storage: storage,
		ui:      ui,
	}
}

func (b *browser) ListAndBrowse(ctx context.Context, groupID, topicID int64) error {
	for {
		index, _, indexed, err := b.storage.GetIndex(ctx, groupID, topicID)
		if err != nil {
			return fmt.Errorf("failed to read index: %w", err)
		}
		var files []domain.RemoteFile
		if indexed {
			files = index.RemoteFiles()
		} else {
			confirmed, err := b.ui.ConfirmCreateIndex("Topic index not found. Create it now?")
			if err != nil {
				return err
			}
			if confirmed {
				if _, err := MigrateTopicIndex(ctx, b.storage, groupID, topicID); err != nil {
					return err
				}
				index, _, _, err = b.storage.GetIndex(ctx, groupID, topicID)
				if err != nil {
					return fmt.Errorf("failed to read new index: %w", err)
				}
				files = index.RemoteFiles()
			} else {
				files, err = b.storage.ListFiles(ctx, groupID, topicID)
				if err != nil {
					return fmt.Errorf("failed to list files: %w", err)
				}
			}
		}

		if len(files) == 0 {
			return fmt.Errorf("no files found in this topic")
		}

		res, err := b.ui.BrowseFiles(files)
		if err != nil {
			return err
		}

		if req, ok := res.(*domain.DownloadRequest); ok {
			return &domain.NavigationError{Type: "download", Data: req}
		}

		return nil
	}
}
