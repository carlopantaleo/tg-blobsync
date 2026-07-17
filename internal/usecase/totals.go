package usecase

import (
	"context"
	"fmt"
	"tg-blobsync/internal/domain"
)

// TotalsUI defines the interface required by the group totals use case.
type TotalsUI interface {
	ConfirmCreateIndex(message string) (bool, error)
	ShowGroupTotals(totals domain.GroupTotals) error
}

// GroupTotalsCalculator computes group totals using topic indexes when available,
// prompting the user to create indexes for topics that lack one.
type GroupTotalsCalculator struct {
	storage domain.BlobStorage
	ui      TotalsUI
}

// NewGroupTotalsCalculator creates a new GroupTotalsCalculator.
func NewGroupTotalsCalculator(storage domain.BlobStorage, ui TotalsUI) *GroupTotalsCalculator {
	return &GroupTotalsCalculator{storage: storage, ui: ui}
}

// Compute calculates and displays group totals.
func (c *GroupTotalsCalculator) Compute(ctx context.Context, groupID int64) (domain.GroupTotals, error) {
	topics, err := c.storage.ListTopics(ctx, groupID)
	if err != nil {
		return domain.GroupTotals{}, fmt.Errorf("failed to list topics: %w", err)
	}

	var indexedTopics, nonIndexedTopics []domain.Topic
	for _, topic := range topics {
		_, _, hasIndex, err := c.storage.GetIndex(ctx, groupID, topic.ID)
		if err != nil {
			return domain.GroupTotals{}, fmt.Errorf("failed to check index for topic %d: %w", topic.ID, err)
		}
		if hasIndex {
			indexedTopics = append(indexedTopics, topic)
		} else {
			nonIndexedTopics = append(nonIndexedTopics, topic)
		}
	}

	if len(nonIndexedTopics) > 0 {
		confirmed, err := c.ui.ConfirmCreateIndex(fmt.Sprintf("%d topic(s) without index found. Create indexes now?", len(nonIndexedTopics)))
		if err != nil {
			return domain.GroupTotals{}, err
		}
		if confirmed {
			for _, topic := range nonIndexedTopics {
				if _, err := MigrateTopicIndex(ctx, c.storage, groupID, topic.ID); err != nil {
					return domain.GroupTotals{}, err
				}
			}
			indexedTopics = append(indexedTopics, nonIndexedTopics...)
			nonIndexedTopics = nil
		}
	}

	var totals domain.GroupTotals
	for _, topic := range indexedTopics {
		index, _, _, err := c.storage.GetIndex(ctx, groupID, topic.ID)
		if err != nil {
			return totals, err
		}
		if index != nil {
			totals.Files += len(index.Entries)
			for _, entry := range index.Entries {
				totals.TotalSize += entry.Size
			}
		}
	}
	for _, topic := range nonIndexedTopics {
		files, err := c.storage.ListFiles(ctx, groupID, topic.ID)
		if err != nil {
			return totals, err
		}
		for _, file := range files {
			totals.Files++
			totals.TotalSize += file.Size
		}
	}
	return totals, nil
}
