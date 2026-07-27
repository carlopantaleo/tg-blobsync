package telegram

import (
	"testing"
	"tg-blobsync/internal/domain"
)

func TestChunkPlanSplitsOnlyAboveThreshold(t *testing.T) {
	if got := chunkPlan(100, 100, 40); len(got) != 0 {
		t.Fatalf("chunk plan at threshold = %#v, want no chunks", got)
	}
	got := chunkPlan(101, 100, 40)
	want := []chunkRange{{Offset: 0, Length: 40}, {Offset: 40, Length: 40}, {Offset: 80, Length: 21}}
	if len(got) != len(want) {
		t.Fatalf("chunk plan = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunk %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestChunkPlanEmptyFile(t *testing.T) {
	if got := chunkPlan(0, 0, 10); len(got) != 0 {
		t.Fatalf("empty file plan = %#v, want no chunks", got)
	}
}

func TestGroupChunkFilesWithoutChecksum(t *testing.T) {
	files := []domain.RemoteFile{
		{Meta: domain.FileMeta{Path: "large.bin", Flags: domain.ChunkFlag, Idx: 1, ModTime: 42}, MessageID: 12, Size: 3},
		{Meta: domain.FileMeta{Path: "large.bin", Flags: domain.ChunkFlag, Idx: 0, ModTime: 42}, MessageID: 11, Size: 4},
	}
	got := groupChunkFiles(files)
	if len(got) != 1 || got[0].Size != 7 {
		t.Fatalf("grouped files = %#v", got)
	}
	if len(got[0].ChunkIDs) != 2 || got[0].ChunkIDs[0] != 11 || got[0].ChunkIDs[1] != 12 {
		t.Fatalf("chunk IDs = %#v", got[0].ChunkIDs)
	}
}

func TestGroupChunkFilesSkipsIncompleteSet(t *testing.T) {
	files := []domain.RemoteFile{{Meta: domain.FileMeta{Path: "large.bin", Flags: domain.ChunkFlag, Idx: 1}, MessageID: 12, Size: 3}}
	if got := groupChunkFiles(files); len(got) != 0 {
		t.Fatalf("grouped incomplete files = %#v", got)
	}
}
