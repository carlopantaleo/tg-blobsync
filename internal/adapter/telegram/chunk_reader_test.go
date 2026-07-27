package telegram

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestChunkReaderConcatenatesLazily(t *testing.T) {
	opened := make([]int, 0)
	reader := newChunkReader([]int{1, 2}, func(id int) (io.ReadCloser, error) {
		opened = append(opened, id)
		return io.NopCloser(strings.NewReader(map[int]string{1: "abc", 2: "def"}[id])), nil
	})
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read chunks: %v", err)
	}
	if string(data) != "abcdef" {
		t.Fatalf("data = %q, want %q", data, "abcdef")
	}
	if len(opened) != 2 || opened[0] != 1 || opened[1] != 2 {
		t.Fatalf("opened chunks = %#v", opened)
	}
}

func TestChunkReaderReturnsOpenError(t *testing.T) {
	wantErr := errors.New("missing chunk")
	reader := newChunkReader([]int{1}, func(int) (io.ReadCloser, error) { return nil, wantErr })
	_, err := io.ReadAll(reader)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}
