## 1. Setup and Metadata Serialization

- [x] 1.1 Create and checkout feature branch `fix/index-read-and-chunk-metadata`
- [x] 1.2 Write test for chunk indexing in `internal/adapter/telegram/files_test.go` (or related file) to ensure chunks start at 1 instead of 0
- [x] 1.3 Update chunk generation logic (`uploadChunk` loops) to start `Idx` at 1
- [x] 1.4 Update chunk reassembly and progress tracking logic to account for 1-based index
- [x] 1.5 Commit progress: `git commit -m "feat: use 1-based indexing for file chunks"`

## 2. Centralize Index Read in Interactive Path Resolution

- [x] 2.1 Write/Update tests in `cmd/tgblobsync/main_test.go` (or related test files) to assert `resolveIdentifiersInternal` uses `GetIndex` when building the sub-directory list
- [x] 2.2 Modify `resolveIdentifiersInternal` in `cmd/tgblobsync/main.go` to attempt `blobStorage.GetIndex` before falling back to `storage.ListFiles` for sub-directory selection
- [x] 2.3 Verify fallback behavior works smoothly without prompting the user for index creation
- [x] 2.4 Commit progress: `git commit -m "feat: use topic index for interactive dir selection"`