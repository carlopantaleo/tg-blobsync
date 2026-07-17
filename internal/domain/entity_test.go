package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestFileIndexFromRemoteFiles(t *testing.T) {
	files := []RemoteFile{{
		Meta:      FileMeta{Path: "documents/report.txt", Checksum: "checksum", ModTime: 1234567890, Flags: EmptyFileFlag},
		MessageID: 42,
		Size:      0,
	}}

	index := NewFileIndex(files)

	want := FileIndex{Entries: []FileIndexEntry{{
		Path:      "documents/report.txt",
		Checksum:  "checksum",
		ModTime:   1234567890,
		Flags:     EmptyFileFlag,
		Size:      0,
		MessageID: 42,
	}}}
	if !reflect.DeepEqual(index, want) {
		t.Fatalf("index = %#v, want %#v", index, want)
	}

	if got := index.RemoteFiles(); !reflect.DeepEqual(got, files) {
		t.Fatalf("remote files = %#v, want %#v", got, files)
	}
}

func TestFileIndexJSONRoundTrip(t *testing.T) {
	index := FileIndex{Entries: []FileIndexEntry{{
		Path:      "documents/report.txt",
		Checksum:  "checksum",
		ModTime:   1234567890,
		Flags:     EmptyFileFlag,
		Size:      0,
		MessageID: 42,
	}}}

	data, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("marshal index: %v", err)
	}

	var decoded FileIndex
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}

	if !reflect.DeepEqual(decoded, index) {
		t.Fatalf("decoded index = %#v, want %#v", decoded, index)
	}
}
