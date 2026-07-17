package telegram

import (
	"encoding/json"
	"testing"

	"tg-blobsync/internal/domain"

	"github.com/gotd/td/tg"
)

func TestAddIndexTotals(t *testing.T) {
	var totals domain.GroupTotals
	addIndexTotals(&totals, domain.FileIndex{Entries: []domain.FileIndexEntry{{Size: 10}, {Size: 20}}})
	if totals.Files != 2 || totals.TotalSize != 30 {
		t.Fatalf("totals = %#v, want 2 files and 30 bytes", totals)
	}
}

func TestParseMessageToFileNormalizesEmptyFileSize(t *testing.T) {
	caption, err := json.Marshal(domain.FileMeta{Path: "empty.txt", Checksum: "checksum", Flags: domain.EmptyFileFlag})
	if err != nil {
		t.Fatalf("marshal caption: %v", err)
	}
	message := &tg.Message{
		ID:      42,
		Message: string(caption),
		Media:   &tg.MessageMediaDocument{Document: &tg.Document{Size: 1}},
	}

	file, ok := (&TelegramClient{}).parseMessageToFile(message, 0)
	if !ok {
		t.Fatal("expected empty file message to be parsed")
	}
	if file.Size != 0 {
		t.Fatalf("empty file size = %d, want 0", file.Size)
	}
}
