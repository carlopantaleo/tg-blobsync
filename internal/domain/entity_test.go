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

func TestChunkMetadataRoundTrip(t *testing.T) {
	meta := FileMeta{Path: "large.bin", ModTime: 42, Flags: ChunkFlag, Idx: 3}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	var decoded FileMeta
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if !reflect.DeepEqual(decoded, meta) {
		t.Fatalf("decoded metadata = %#v, want %#v", decoded, meta)
	}
}

func TestFileIndexRoundTripChunkIDs(t *testing.T) {
	files := []RemoteFile{{
		Meta:      FileMeta{Path: "large.bin", Flags: ChunkFlag},
		MessageID: 10,
		ChunkIDs:  []int{10, 11, 12},
		Size:      7,
	}}
	index := NewFileIndex(files)
	if !reflect.DeepEqual(index.Entries[0].ChunkIDs, files[0].ChunkIDs) {
		t.Fatalf("chunk IDs = %#v, want %#v", index.Entries[0].ChunkIDs, files[0].ChunkIDs)
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
