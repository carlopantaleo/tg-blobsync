package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"

	"tg-blobsync/internal/adapter/ui"
	"tg-blobsync/internal/config"
	"tg-blobsync/internal/domain"
)

func TestImmediateSubDirs(t *testing.T) {
	files := []domain.RemoteFile{
		{Meta: domain.FileMeta{Path: "a/b/c/file1.txt"}},
		{Meta: domain.FileMeta{Path: "a/file2.txt"}},
		{Meta: domain.FileMeta{Path: "other/f.txt"}},
		{Meta: domain.FileMeta{Path: "root.txt"}},
		{Meta: domain.FileMeta{Path: "a/b/file3.txt"}},
	}

	tests := []struct {
		name     string
		prefix   string
		expected []string
	}{
		{"root lists first-level dirs only", "", []string{"a", "other"}},
		{"a lists its immediate children", "a", []string{"b"}},
		{"a/b lists its immediate children", "a/b", []string{"c"}},
		{"leaf dir yields empty list", "a/b/c", nil},
		{"non-matching prefix yields empty list", "zzz", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := immediateSubDirs(files, tt.prefix)
			if len(got) == 0 && len(tt.expected) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("immediateSubDirs(%q) = %v, want %v", tt.prefix, got, tt.expected)
			}
		})
	}
}

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
	blobStorage := &stubBlobStorage{}
	console := &stubConsole{subdirSelections: []ui.SubDirSelection{{Action: ui.SubDirEnter, Value: "chosen"}, {Action: ui.SubDirThis}}}

	groupID, topicID, err := resolveIdentifiersInternal(ctx, cfg, storage, blobStorage, console)
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
	blobStorage := &stubBlobStorage{}
	console := &stubConsole{subdirSelections: []ui.SubDirSelection{{Action: ui.SubDirThis}}}

	groupID, topicID, err := resolveIdentifiersInternal(ctx, cfg, storage, blobStorage, console)
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
	subdirSelections   []ui.SubDirSelection
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

func (s *stubConsole) SelectSubDir(_ []string, _ string) (ui.SubDirSelection, error) {
	s.selectSubdirCalled = true
	if s.subdirErr != nil {
		return ui.SubDirSelection{}, s.subdirErr
	}
	sel := s.subdirSelections[0]
	s.subdirSelections = s.subdirSelections[1:]
	return sel, nil
}

func (s *stubConsole) ShowGroupTotals(_ domain.GroupTotals) error {
	return nil
}

func (s *stubConsole) ConfirmCreateIndex(_ string) (bool, error) {
	return false, nil
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

func (s *stubTelegram) GroupTotals(_ context.Context, _ int64) (domain.GroupTotals, error) {
	return domain.GroupTotals{}, nil
}

type stubBlobStorage struct {
	index *domain.FileIndex
}

func (s *stubBlobStorage) GetIndex(ctx context.Context, groupID int64, topicID int64) (*domain.FileIndex, int, bool, error) {
	if s.index != nil {
		return s.index, 1, true, nil
	}
	return nil, 0, false, nil
}

func (s *stubBlobStorage) UploadIndex(ctx context.Context, groupID int64, topicID int64, index domain.FileIndex) (int, error) {
	return 0, nil
}

func (s *stubBlobStorage) DownloadFile(ctx context.Context, groupID int64, topicID int64, messageID int, fileName string, size int64, task domain.ProgressTask) (io.ReadCloser, error) {
	return nil, nil
}

func (s *stubBlobStorage) DownloadChunkedFile(ctx context.Context, groupID, topicID int64, chunkIDs []int, fileName string, size int64, task domain.ProgressTask) (io.ReadCloser, error) {
	return nil, nil
}

func (s *stubBlobStorage) UploadFile(ctx context.Context, groupID int64, topicID int64, file domain.LocalFile) ([]int, error) {
	return []int{1}, nil
}

func (s *stubBlobStorage) UploadChunkedFile(ctx context.Context, groupID, topicID int64, file domain.LocalFile) error {
	return nil
}

func (s *stubBlobStorage) DeleteFile(ctx context.Context, groupID int64, topicID int64, messageID int) error {
	return nil
}

func (s *stubBlobStorage) DeleteChunkedFile(ctx context.Context, groupID, topicID int64, chunkIDs []int) error {
	return nil
}

func (s *stubBlobStorage) ListFiles(ctx context.Context, groupID int64, topicID int64) ([]domain.RemoteFile, error) {
	return nil, nil
}

func (s *stubBlobStorage) ListIndexMessageIDs(ctx context.Context, groupID int64, topicID int64) ([]int, error) {
	return nil, nil
}

func (s *stubBlobStorage) Close() error {
	return nil
}

func (s *stubBlobStorage) GroupTotals(ctx context.Context, groupID int64) (domain.GroupTotals, error) {
	return domain.GroupTotals{}, nil
}

func (s *stubBlobStorage) ListGroups(ctx context.Context) ([]domain.Group, error) {
	return nil, nil
}

func (s *stubBlobStorage) ListTopics(ctx context.Context, groupID int64) ([]domain.Topic, error) {
	return nil, nil
}

func (s *stubBlobStorage) SetProgressTracker(t domain.ProgressTracker) {}
func (s *stubBlobStorage) Start(ctx context.Context) error             { return nil }

func TestResolveIdentifiers_SubDirUsesIndex(t *testing.T) {
	ctx := context.Background()
	cfg := &config.CLIConfig{Command: "push", GroupName: "1", TopicName: "2"}
	storage := &stubTelegram{}
	blobStorage := &stubBlobStorage{
		index: &domain.FileIndex{
			Entries: []domain.FileIndexEntry{
				{Path: "indexed_dir/file.txt"},
			},
		},
	}
	console := &stubConsole{subdirSelections: []ui.SubDirSelection{{Action: ui.SubDirEnter, Value: "indexed_dir"}, {Action: ui.SubDirThis}}}

	groupID, topicID, err := resolveIdentifiersInternal(ctx, cfg, storage, blobStorage, console)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if groupID != 1 || topicID != 2 {
		t.Fatalf("unexpected ids: %d %d", groupID, topicID)
	}
	if cfg.SubDir != "indexed_dir" {
		t.Fatalf("expected subdir 'indexed_dir', got %q", cfg.SubDir)
	}
	if storage.listFilesCalled {
		t.Fatalf("expected ListFiles not to be called when index is present")
	}
	if !console.selectSubdirCalled {
		t.Fatalf("expected selectSubdir to be called")
	}
}

func TestResolveIdentifiers_GroupNotFound(t *testing.T) {
	ctx := context.Background()
	cfg := &config.CLIConfig{Command: "push", GroupName: "Unknown"}
	storage := &stubTelegram{}
	blobStorage := &stubBlobStorage{}
	console := &stubConsole{}

	_, _, err := resolveIdentifiersInternal(ctx, cfg, storage, blobStorage, console)
	if err == nil {
		t.Fatalf("expected error when group not found")
	}
}

func TestResolveIdentifiers_TopicNotFound(t *testing.T) {
	ctx := context.Background()
	cfg := &config.CLIConfig{Command: "push", GroupName: "MyGroup", TopicName: "Missing"}
	storage := &stubTelegram{groupByName: &domain.Group{ID: 7, Title: "MyGroup"}}
	blobStorage := &stubBlobStorage{}
	console := &stubConsole{}

	_, _, err := resolveIdentifiersInternal(ctx, cfg, storage, blobStorage, console)
	if err == nil {
		t.Fatalf("expected error when topic not found")
	}
}

func subdirDrillDownStorage() *stubTelegram {
	return &stubTelegram{
		files: []domain.RemoteFile{
			{Meta: domain.FileMeta{Path: "a/b/c/file1.txt"}},
			{Meta: domain.FileMeta{Path: "a/file2.txt"}},
			{Meta: domain.FileMeta{Path: "other/f.txt"}},
		},
	}
}

func TestResolveIdentifiers_DrillDownIntoNestedSubdir(t *testing.T) {
	ctx := context.Background()
	cfg := &config.CLIConfig{Command: "push", GroupName: "1", TopicName: "2"}
	storage := subdirDrillDownStorage()
	blobStorage := &stubBlobStorage{}
	console := &stubConsole{subdirSelections: []ui.SubDirSelection{
		{Action: ui.SubDirEnter, Value: "a"},
		{Action: ui.SubDirEnter, Value: "b"},
		{Action: ui.SubDirThis},
	}}

	_, _, err := resolveIdentifiersInternal(ctx, cfg, storage, blobStorage, console)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SubDir != "a/b" {
		t.Fatalf("expected subdir 'a/b', got %q", cfg.SubDir)
	}
}

func TestResolveIdentifiers_CustomPathRelativeToCurrentDir(t *testing.T) {
	ctx := context.Background()
	cfg := &config.CLIConfig{Command: "push", GroupName: "1", TopicName: "2"}
	storage := subdirDrillDownStorage()
	blobStorage := &stubBlobStorage{}
	console := &stubConsole{subdirSelections: []ui.SubDirSelection{
		{Action: ui.SubDirEnter, Value: "a"},
		{Action: ui.SubDirCustom, Value: "x/y"},
	}}

	_, _, err := resolveIdentifiersInternal(ctx, cfg, storage, blobStorage, console)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SubDir != "a/x/y" {
		t.Fatalf("expected subdir 'a/x/y', got %q", cfg.SubDir)
	}
}

func TestResolveIdentifiers_UpActionAtRootGoesBack(t *testing.T) {
	ctx := context.Background()
	cfg := &config.CLIConfig{Command: "push", GroupName: "1", TopicName: "2"}
	storage := subdirDrillDownStorage()
	blobStorage := &stubBlobStorage{}
	console := &stubConsole{subdirSelections: []ui.SubDirSelection{
		{Action: ui.SubDirUp},
	}}

	_, _, err := resolveIdentifiersInternal(ctx, cfg, storage, blobStorage, console)
	if err == nil || err.Error() != "back" {
		t.Fatalf("expected back error, got %v", err)
	}
}

func TestResolveIdentifiers_UpActionInsideDirGoesUpOneLevel(t *testing.T) {
	ctx := context.Background()
	cfg := &config.CLIConfig{Command: "push", GroupName: "1", TopicName: "2"}
	storage := subdirDrillDownStorage()
	blobStorage := &stubBlobStorage{}
	console := &stubConsole{subdirSelections: []ui.SubDirSelection{
		{Action: ui.SubDirEnter, Value: "a"},
		{Action: ui.SubDirEnter, Value: "b"},
		{Action: ui.SubDirUp},
		{Action: ui.SubDirThis},
	}}

	_, _, err := resolveIdentifiersInternal(ctx, cfg, storage, blobStorage, console)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SubDir != "a" {
		t.Fatalf("expected subdir 'a', got %q", cfg.SubDir)
	}
}

func TestResolveIdentifiers_BackFromSubdir(t *testing.T) {
	ctx := context.Background()
	cfg := &config.CLIConfig{Command: "pull", GroupName: "1", TopicName: "2"}
	storage := &stubTelegram{files: []domain.RemoteFile{{Meta: domain.FileMeta{Path: "d/f"}, Size: 1}}}
	blobStorage := &stubBlobStorage{}
	console := &stubConsole{subdirErr: fmt.Errorf("back")}

	_, _, err := resolveIdentifiersInternal(ctx, cfg, storage, blobStorage, console)
	if err == nil || err.Error() != "back" {
		t.Fatalf("expected back error, got %v", err)
	}
}
