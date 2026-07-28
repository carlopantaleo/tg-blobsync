package telegram

import (
	"io"
	"tg-blobsync/internal/domain"
)

type chunkReader struct {
	open    func(int) (io.ReadCloser, error)
	ids     []int
	task    domain.ProgressTask
	current io.ReadCloser
	index   int
}

func newChunkReader(ids []int, open func(int) (io.ReadCloser, error)) *chunkReader {
	return &chunkReader{ids: ids, open: open}
}

func newChunkReaderWithTask(ids []int, open func(int) (io.ReadCloser, error), task domain.ProgressTask) *chunkReader {
	return &chunkReader{ids: ids, open: open, task: task}
}

func (r *chunkReader) Read(p []byte) (int, error) {
	for {
		if r.current == nil {
			if r.index == len(r.ids) {
				return 0, io.EOF
			}
			if r.task != nil {
				r.task.SetChunk(r.index+1, len(r.ids))
			}
			current, err := r.open(r.ids[r.index])
			if err != nil {
				return 0, err
			}
			r.current = current
			r.index++
		}

		n, err := r.current.Read(p)
		if err != io.EOF {
			return n, err
		}
		_ = r.current.Close()
		r.current = nil
		if n > 0 {
			return n, nil
		}
	}
}

func (r *chunkReader) Close() error {
	if r.current == nil {
		return nil
	}
	err := r.current.Close()
	r.current = nil
	return err
}
