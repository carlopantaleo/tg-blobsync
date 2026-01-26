package usecase

import (
	"context"
	"tg-blobsync/internal/domain"
)

type Selector struct {
	storage domain.BlobStorage
}

func NewSelector(storage domain.BlobStorage) *Selector {
	return &Selector{
		storage: storage,
	}
}

// SelectGroup lists groups and allows selection.
// In a real interactive CLI, the UI adapter would likely handle the interaction,
// but the usecase provides the data.
// For now, let's just return the list.
func (s *Selector) ListGroups(ctx context.Context) ([]domain.Group, error) {
	return s.storage.ListGroups(ctx)
}

func (s *Selector) ListTopics(ctx context.Context, groupID int64) ([]domain.Topic, error) {
	return s.storage.ListTopics(ctx, groupID)
}
