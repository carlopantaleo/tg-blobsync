package main

import (
	"context"
	"fmt"
	"os"
	"testing"

	"tg-blobsync/internal/config"
	"tg-blobsync/internal/domain"
)

// TestRun_NoArgs verifies run returns an error when CLI args are missing.
func TestRun_NoArgs(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"tgblobsync"}

	if err := run(); err == nil {
		t.Fatalf("expected error on missing args")
	}
}

func TestResolveIdentifiersWithNames(t *testing.T) {
	ctx := context.Background()
	cfg := &config.CLIConfig{Command: "push", GroupName: "MyGroup", TopicName: "MyTopic"}
	storage := &stubTelegram{
		groupByName: &domain.Group{ID: 42, Title: "MyGroup"},
		topicByName: &domain.Topic{ID: 99, Title: "MyTopic"},
		files:       []domain.RemoteFile{{Meta: domain.FileMeta{Path: "subdir/file.txt"}, Size: 10}},
	}
	console := &stubConsole{subdir: "chosen"}

	groupID, topicID, err := resolveIdentifiersInternal(ctx, cfg, storage, console)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if groupID != 42 || topicID != 99 {
		t.Fatalf("unexpected ids: %d %d", groupID, topicID)
	}
	if cfg.SubDir != "chosen" {
		t.Fatalf("expected subdir 'chosen', got %q", cfg.SubDir)
	}
	if !storage.findGroupByNameCalled || !storage.findTopicByNameCalled || !storage.listFilesCalled || !console.selectSubdirCalled {
		t.Fatalf("expected name resolution and subdir selection to be called")
	}
}

func TestResolveIdentifiersWithIDs(t *testing.T) {
	ctx := context.Background()
	cfg := &config.CLIConfig{Command: "pull", GroupName: "123", TopicName: "456"}
	storage := &stubTelegram{
		files: []domain.RemoteFile{{Meta: domain.FileMeta{Path: "dir/file.txt"}, Size: 5}},
	}
	console := &stubConsole{subdir: ""}

	groupID, topicID, err := resolveIdentifiersInternal(ctx, cfg, storage, console)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if groupID != 123 || topicID != 456 {
		t.Fatalf("unexpected ids: %d %d", groupID, topicID)
	}
	if !storage.resolveGroupCalled {
		t.Fatalf("expected resolveGroup to be called for numeric id")
	}
	if storage.findGroupByNameCalled || storage.findTopicByNameCalled {
		t.Fatalf("did not expect find by name for numeric ids")
	}
}

type stubConsole struct {
	group              domain.Group
	topic              domain.Topic
	subdir             string
	selectSubdirCalled bool
	groupErr           error
	topicErr           error
	subdirErr          error
}

func (s *stubConsole) SelectGroup(_ []domain.Group) (domain.Group, error) {
	return s.group, s.groupErr
}

func (s *stubConsole) SelectTopic(_ []domain.Topic) (domain.Topic, error) {
	return s.topic, s.topicErr
}

func (s *stubConsole) SelectSubDir(_ []string) (string, error) {
	s.selectSubdirCalled = true
	return s.subdir, s.subdirErr
}

type stubTelegram struct {
	groupByName           *domain.Group
	topicByName           *domain.Topic
	files                 []domain.RemoteFile
	findGroupByNameCalled bool
	findTopicByNameCalled bool
	resolveGroupCalled    bool
	listFilesCalled       bool
}

func (s *stubTelegram) ResolveGroup(_ context.Context, groupID int64) error {
	s.resolveGroupCalled = true
	if groupID == 0 {
		return context.Canceled
	}
	return nil
}

func (s *stubTelegram) FindGroupByName(_ context.Context, _ string) (*domain.Group, error) {
	s.findGroupByNameCalled = true
	return s.groupByName, nil
}

func (s *stubTelegram) FindTopicByName(_ context.Context, _ int64, _ string) (*domain.Topic, error) {
	s.findTopicByNameCalled = true
	return s.topicByName, nil
}

func (s *stubTelegram) ListGroups(_ context.Context) ([]domain.Group, error) {
	if s.groupByName == nil {
		return nil, nil
	}
	return []domain.Group{*s.groupByName}, nil
}

func (s *stubTelegram) ListTopics(_ context.Context, _ int64) ([]domain.Topic, error) {
	if s.topicByName == nil {
		return nil, nil
	}
	return []domain.Topic{*s.topicByName}, nil
}

func (s *stubTelegram) ListFiles(_ context.Context, _ int64, _ int64) ([]domain.RemoteFile, error) {
	s.listFilesCalled = true
	return s.files, nil
}

func TestResolveIdentifiers_GroupNotFound(t *testing.T) {
	ctx := context.Background()
	cfg := &config.CLIConfig{Command: "push", GroupName: "Unknown"}
	storage := &stubTelegram{}
	console := &stubConsole{}

	_, _, err := resolveIdentifiersInternal(ctx, cfg, storage, console)
	if err == nil {
		t.Fatalf("expected error when group not found")
	}
}

func TestResolveIdentifiers_TopicNotFound(t *testing.T) {
	ctx := context.Background()
	cfg := &config.CLIConfig{Command: "push", GroupName: "MyGroup", TopicName: "Missing"}
	storage := &stubTelegram{groupByName: &domain.Group{ID: 7, Title: "MyGroup"}}
	console := &stubConsole{}

	_, _, err := resolveIdentifiersInternal(ctx, cfg, storage, console)
	if err == nil {
		t.Fatalf("expected error when topic not found")
	}
}

func TestResolveIdentifiers_BackFromSubdir(t *testing.T) {
	ctx := context.Background()
	cfg := &config.CLIConfig{Command: "pull", GroupName: "1", TopicName: "2"}
	storage := &stubTelegram{files: []domain.RemoteFile{{Meta: domain.FileMeta{Path: "d/f"}, Size: 1}}}
	console := &stubConsole{subdirErr: fmt.Errorf("back")}

	_, _, err := resolveIdentifiersInternal(ctx, cfg, storage, console)
	if err == nil || err.Error() != "back" {
		t.Fatalf("expected back error, got %v", err)
	}
}
