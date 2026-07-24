package telegram

import "testing"

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
