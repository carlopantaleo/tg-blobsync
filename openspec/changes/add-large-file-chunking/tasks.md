## 1. Branch and configuration

- [x] 1.1 Create branch `feat/large-file-chunking` from the current main
- [x] 1.2 Add `chunkThreshold` (default 2 GB) and `chunkSize` (default 1 GB) fields to `internal/config` with JSON/yaml tags
- [x] 1.3 Add config validation: `chunkSize > 0` and `chunkSize <= chunkThreshold`; fail fast on invalid values
- [x] 1.4 Wire the chunk settings into the Telegram client constructor

## 2. Domain model (TDD)

- [x] 2.1 Add `ChunkFlag = "CHUNK"` constant and `Idx int` field to `FileMeta` (`json:"i,omitempty"`)
- [x] 2.2 Add `ChunkIDs []int` field to `FileIndexEntry` (`json:"c,omitempty"`) and to `RemoteFile`
- [x] 2.3 Update `NewFileIndex`/`FileIndex.RemoteFiles()` to round-trip `ChunkIDs`
- [x] 2.4 Write unit tests covering: `FileMeta` marshal/unmarshal with `Idx`; `FileIndex` round-trip with `chunkIDs`; chunked entry recovery preserves order and total size
- [x] 2.5 Extend `ProgressTask` interface with optional chunk info (e.g. `SetChunk(current, total int)`) keeping backward compatibility for non-chunked files (callers may skip it)

## 3. Chunk split/assemble helpers (TDD)

- [x] 3.1 Implement `chunkPlan(size, threshold, chunkSize) []chunkRange` returning ordered `(offset, length)` ranges; pure function
- [x] 3.2 Tests: file at threshold is not chunked; file above threshold splits into `ceil(size/chunkSize)` ranges; last chunk is smaller; empty file yields no chunks
- [ ] 3.3 Implement a streaming chunk reader (`chunkReader`) that opens the next chunk lazily and concatenates via `io.MultiReader`-style chaining without buffering whole chunks
- [ ] 3.4 Tests: concatenated reader yields exact original bytes for a multi-chunk fixture; missing chunk returns error; reader closes underlying chunk readers on close

## 4. Telegram adapter - upload (TDD)

- [ ] 4.1 Refactor `UploadFile` to branch on `file.Size > chunkThreshold`
- [ ] 4.2 Implement chunked upload: split via `chunkPlan`, upload each chunk with `Flags=CHUNK` and `Idx`, collect returned message IDs
- [ ] 4.3 Aggregate progress across chunks (single `ProgressTask` with total = logical size) and expose current chunk index + total chunk count for per-chunk UI (`chunk i/N`)
- [ ] 4.4 On upload error, best-effort delete the chunks already uploaded in the attempt
- [ ] 4.5 Return the new `chunkIDs` (extend `UploadFile` signature or add `UploadFileChunked` returning `[]int`)
- [ ] 4.6 Tests with mock invoker: 3-chunk upload produces 3 messages with increasing `Idx` and correct captions; failure mid-upload deletes uploaded chunks

## 5. Telegram adapter - download (TDD)

- [ ] 5.1 Extend `DownloadFile` to accept `chunkIDs []int` (add overload or extend signature) and stream chunks in `Idx` order
- [ ] 5.2 Implement lazy chunk reader chaining on top of the existing per-message downloader; expose current chunk index + total chunk count for per-chunk UI
- [ ] 5.3 Tests: multi-chunk download yields concatenated bytes in order; missing chunk ID returns error and no partial file

## 6. Telegram adapter - delete and parse (TDD)

- [ ] 6.1 Extend `DeleteFile` to accept `chunkIDs []int` and delete all of them
- [ ] 6.2 Update `parseMessageToFile` to parse `Idx` and `CHUNK` flag
- [ ] 6.3 Update `ListFiles` legacy fallback to group `CHUNK` messages into one `RemoteFile` with `ChunkIDs` sorted by `Idx`; grouping key is `Path`+`Checksum` when `Checksum` is non-empty, else `Path` alone (covers `--skip-md5`); skip incomplete sets
- [ ] 6.4 Tests: fallback groups chunks into one `RemoteFile` both with and without `Checksum`; orphan/incomplete chunk set is skipped

## 7. Topic index integration (TDD)

- [ ] 7.1 `UploadIndex` serializes `chunkIDs` for chunked entries
- [ ] 7.2 `GetIndex` recovery builds `RemoteFile` with `ChunkIDs` and total size for chunked entries
- [ ] 7.3 Tests: index round-trip with a mix of non-chunked and chunked entries preserves `chunkIDs` order and sizes

## 8. Use cases - differ and executor (TDD)

- [ ] 8.1 Confirm `differ` already compares logical size (no change needed) and add tests for chunked push/pull plans counting the file once
- [ ] 8.2 Update `executor.upload` to call chunked upload and, on update, delete all old `ChunkIDs` after new chunks are uploaded
- [ ] 8.3 Update `executor.download` to pass `chunkIDs` to chunked download
- [ ] 8.4 Update `executor.deleteRemote` to delete all `ChunkIDs`
- [ ] 8.5 Tests with mocks: chunked upload/update/download/delete operate on the whole logical file

## 9. TUI per-chunk progress

- [x] 9.1 Render per-chunk indicator (`chunk i/N`) in the console/TUI progress task when chunk info is set, alongside the aggregated byte progress
- [ ] 9.2 Tests: progress task with chunk info renders the chunk indicator; non-chunked tasks render unchanged

## 10. Index rebuild after sync

- [ ] 10.1 Ensure post-sync index rebuild includes `chunkIDs` for chunked files (verify existing rebuild path picks up the new field via `NewFileIndex`)
- [ ] 10.2 Test: after a chunked upload, the rebuilt index entry has correct `chunkIDs` and total size

## 11. End-to-end and regression

- [ ] 11.1 Add integration-style test (mock invoker) for a full push of a >threshold file: chunks uploaded, index rebuilt with `chunkIDs`, subsequent pull reassembles the file
- [ ] 11.2 Regression: existing single-message upload/download/index tests still pass unchanged
- [ ] 11.3 Run `go vet ./...` and `go test ./...` and fix any failures

## 12. Documentation and commit hygiene

- [ ] 12.1 Update `README.md` with a short note on large file support and the new config fields
- [ ] 12.2 Make a conventional commit per milestone (config, domain, adapter upload, adapter download, adapter delete/parse, index, usecase, tui, e2e, docs)
- [ ] 12.3 Run `openspec validate add-large-file-chunking` and fix any reported issues
