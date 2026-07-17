package usecase

import (
	"context"
	"fmt"
	"tg-blobsync/internal/domain"
)

// MigrateTopicIndex runs the legacy fallback migration for a topic: paginates
// via ListFiles, deletes stale INDEX messages, builds a FileIndex from the
// collected remote files, and uploads it as the last message of the topic.
// Returns the new INDEX message ID.
func MigrateTopicIndex(ctx context.Context, storage domain.BlobStorage, groupID, topicID int64) (int, error) {
	files, err := storage.ListFiles(ctx, groupID, topicID)
	if err != nil {
		return 0, fmt.Errorf("failed to list remote files: %w", err)
	}
	staleIDs, err := storage.ListIndexMessageIDs(ctx, groupID, topicID)
	if err != nil {
		return 0, fmt.Errorf("failed to list stale indexes: %w", err)
	}
	for _, messageID := range staleIDs {
		if err := storage.DeleteFile(ctx, groupID, topicID, messageID); err != nil {
			return 0, fmt.Errorf("failed to delete stale index %d: %w", messageID, err)
		}
	}
	messageID, err := storage.UploadIndex(ctx, groupID, topicID, domain.NewFileIndex(files))
	if err != nil {
		return 0, fmt.Errorf("failed to upload migrated index: %w", err)
	}
	return messageID, nil
}
