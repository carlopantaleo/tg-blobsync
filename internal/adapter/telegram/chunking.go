package telegram

type chunkRange struct {
	Offset int64
	Length int64
}

func chunkPlan(size, threshold, chunkSize int64) []chunkRange {
	if size <= threshold || size <= 0 || chunkSize <= 0 {
		return nil
	}

	chunks := make([]chunkRange, 0, (size+chunkSize-1)/chunkSize)
	for offset := int64(0); offset < size; offset += chunkSize {
		length := chunkSize
		if remaining := size - offset; remaining < length {
			length = remaining
		}
		chunks = append(chunks, chunkRange{Offset: offset, Length: length})
	}
	return chunks
}
