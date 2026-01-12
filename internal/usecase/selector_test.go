package usecase

import (
	"context"
	"testing"
	"tg-blobsync/internal/domain"
)

func TestSelector_ListGroupsAndTopics(t *testing.T) {
	mockStorage := NewMockBlobStorage()
	selector := NewSelector(mockStorage)
	ctx := context.Background()

	// Setup data
	mockStorage.Groups = []domain.Group{{ID: 1, Title: "Group 1"}}
	mockStorage.Topics[1] = []domain.Topic{{ID: 2, Title: "Topic 1"}}

	// Test ListGroups
	groups, err := selector.ListGroups(ctx)
	if err != nil {
		t.Fatalf("ListGroups failed: %v", err)
	}
	if len(groups) != 1 {
		t.Errorf("Expected 1 group, got %d", len(groups))
	}
	if groups[0].ID != 1 {
		t.Errorf("Group ID mismatch")
	}

	// Test ListTopics
	topics, err := selector.ListTopics(ctx, 1)
	if err != nil {
		t.Fatalf("ListTopics failed: %v", err)
	}
	if len(topics) != 1 {
		t.Errorf("Expected 1 topic, got %d", len(topics))
	}
	if topics[0].ID != 2 {
		t.Errorf("Topic ID mismatch")
	}
}
