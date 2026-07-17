package usecase

import (
	"context"
	"testing"
	"tg-blobsync/internal/domain"
)

type mockTotalsUI struct {
	confirmResult  bool
	confirmCalled  bool
	confirmMessage string
	shownTotals    domain.GroupTotals
}

func (m *mockTotalsUI) ConfirmCreateIndex(message string) (bool, error) {
	m.confirmCalled = true
	m.confirmMessage = message
	return m.confirmResult, nil
}

func (m *mockTotalsUI) ShowGroupTotals(totals domain.GroupTotals) error {
	m.shownTotals = totals
	return nil
}

func TestGroupTotalsAllTopicsIndexed(t *testing.T) {
	storage := NewMockBlobStorage()
	storage.Topics[1] = []domain.Topic{{ID: 10}, {ID: 20}}
	storage.Indexes[1] = map[int64]*domain.FileIndex{
		10: {Entries: []domain.FileIndexEntry{{Size: 100}, {Size: 200}}},
		20: {Entries: []domain.FileIndexEntry{{Size: 50}}},
	}
	storage.IndexIDs[1] = map[int64]int{10: 1, 20: 2}
	ui := &mockTotalsUI{}

	totals, err := NewGroupTotalsCalculator(storage, ui).Compute(context.Background(), 1)
	if err != nil {
		t.Fatalf("Compute failed: %v", err)
	}
	if ui.confirmCalled {
		t.Fatal("did not expect prompt when all topics are indexed")
	}
	if totals.Files != 3 || totals.TotalSize != 350 {
		t.Fatalf("totals = %#v, want 3 files and 350 bytes", totals)
	}
}

func TestGroupTotalsPromptsWhenSomeTopicsLackIndex(t *testing.T) {
	storage := NewMockBlobStorage()
	storage.Topics[1] = []domain.Topic{{ID: 10}, {ID: 20}}
	storage.Indexes[1] = map[int64]*domain.FileIndex{
		10: {Entries: []domain.FileIndexEntry{{Size: 100}}},
	}
	storage.IndexIDs[1] = map[int64]int{10: 1}
	storage.Files[1] = map[int64][]domain.RemoteFile{
		20: {{Meta: domain.FileMeta{Path: "file.txt"}, Size: 50}},
	}
	ui := &mockTotalsUI{confirmResult: true}

	totals, err := NewGroupTotalsCalculator(storage, ui).Compute(context.Background(), 1)
	if err != nil {
		t.Fatalf("Compute failed: %v", err)
	}
	if !ui.confirmCalled {
		t.Fatal("expected prompt when some topics lack index")
	}
	if storage.IndexUploads != 1 {
		t.Fatalf("index uploads = %d, want 1 (migration for topic 20)", storage.IndexUploads)
	}
	if totals.Files != 2 || totals.TotalSize != 150 {
		t.Fatalf("totals = %#v, want 2 files and 150 bytes", totals)
	}
}

func TestGroupTotalsDeclineUsesLegacyFallback(t *testing.T) {
	storage := NewMockBlobStorage()
	storage.Topics[1] = []domain.Topic{{ID: 10}, {ID: 20}}
	storage.Indexes[1] = map[int64]*domain.FileIndex{
		10: {Entries: []domain.FileIndexEntry{{Size: 100}}},
	}
	storage.IndexIDs[1] = map[int64]int{10: 1}
	storage.Files[1] = map[int64][]domain.RemoteFile{
		20: {{Meta: domain.FileMeta{Path: "file.txt"}, Size: 50}},
	}
	ui := &mockTotalsUI{confirmResult: false}

	totals, err := NewGroupTotalsCalculator(storage, ui).Compute(context.Background(), 1)
	if err != nil {
		t.Fatalf("Compute failed: %v", err)
	}
	if !ui.confirmCalled {
		t.Fatal("expected prompt when some topics lack index")
	}
	if storage.IndexUploads != 0 {
		t.Fatalf("index uploads = %d, want 0 (declined)", storage.IndexUploads)
	}
	if totals.Files != 2 || totals.TotalSize != 150 {
		t.Fatalf("totals = %#v, want 2 files and 150 bytes", totals)
	}
}
