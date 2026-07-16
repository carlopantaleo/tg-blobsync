## 1. Domain model

- [ ] 1.1 Add `FileIndexEntry` and `FileIndex` structs to `internal/domain/entity.go` (fields: Path, Checksum, ModTime, Flags, Size, MessageID)
- [ ] 1.2 Add the `INDEX` flag constant alongside the existing `EMPTY_FILE` usage
- [ ] 1.3 Write unit tests for `FileIndex` (de)serialization round-trip

## 2. BlobStorage interface

- [ ] 2.1 Add `GetIndex(ctx, groupID, topicID) (*FileIndex, int, bool, error)` to the `BlobStorage` interface in `internal/domain/repository.go`
- [ ] 2.2 Add `UploadIndex(ctx, groupID, topicID, index FileIndex) (int, error)` to the `BlobStorage` interface
- [ ] 2.3 Update all `BlobStorage` mocks/fakes in tests to implement the new methods

## 3. Telegram adapter — index read

- [ ] 3.1 Implement `GetIndex`: fetch the last message of the topic (`MessagesGetReplies` with `limit=1`), detect the `{"f":"INDEX"}` caption, download `index.json`, unmarshal into `FileIndex`
- [ ] 3.2 Write TDD tests for `GetIndex`: index present, index absent (last message is a file), empty topic, malformed index document
- [ ] 3.3 Ensure `parseMessageToFile` skips messages whose caption parses to `Flags == "INDEX"` (they are not files)

## 4. Telegram adapter — index write

- [ ] 4.1 Implement `UploadIndex`: marshal `FileIndex` to JSON, upload as `index.json` document with caption `{"f":"INDEX"}` as the last message of the topic, return the new message ID
- [ ] 4.2 Write TDD tests for `UploadIndex`: successful upload returns a message ID, caption is exactly `{"f":"INDEX"}`

## 5. Telegram adapter — legacy fallback with migration

- [ ] 5.1 Extend `ListFiles` (or add a sibling method) to also return the IDs of any messages whose caption is `{"f":"INDEX"}` (stale indexes), so the caller can clean them up
- [ ] 5.2 In the fallback path: paginate via `ListFiles`, delete every stale INDEX message, build a `FileIndex` from the collected `RemoteFile`s, and upload it as the last message
- [ ] 5.3 Normalize `RemoteFile.Size` to 0 when `Flags == "EMPTY_FILE"` in the legacy path
- [ ] 5.4 Write TDD tests for the fallback: legacy topic (no index) builds and uploads an index; stale INDEX messages are deleted; empty-file size is normalized to 0

## 6. Scanner — use the index

- [ ] 6.1 Update `scanner.ScanRemote` to call `GetIndex` first; on hit, build the remote map from the index entries (still applying the existing `subDir` filtering); on miss, run the legacy fallback flow
- [ ] 6.2 Keep the `subDir` filtering logic identical between the index path and the legacy path
- [ ] 6.3 Write TDD tests for `ScanRemote`: index hit returns filtered entries without pagination; index miss triggers fallback and returns the migrated state

## 7. Differ — remove EMPTY_FILE size special-case

- [ ] 7.1 Remove the `EMPTY_FILE` size special-case from `differ.shouldUpdate`; compare `remote.Size` directly
- [ ] 7.2 Update existing differ tests to reflect the normalized size (empty files now have `Size == 0` from both the index and the legacy path)
- [ ] 7.3 Add differ tests covering empty-file comparison via the index path

## 8. Synchronizer — post-sync rebuild

- [ ] 8.1 After `executor.Execute` in `Synchronizer.Push` and `Pull`, if the plan had changes (`plan.Summary.Total > 0`), delete the previous INDEX message (if known) and upload a fresh `FileIndex` as the last message
- [ ] 8.2 If the plan had no changes, skip the rebuild
- [ ] 8.3 Track the current INDEX message ID through the sync flow (from `GetIndex` or from the fallback migration upload) so the rebuild can delete it
- [ ] 8.4 Write TDD tests: sync with changes rebuilds the index (delete old + upload new); sync with no changes does not touch the index

## 9. Browse — use the index

- [ ] 9.1 Update `browser.ListAndBrowse` to read files from the index when present; fall back to legacy `ListFiles` otherwise
- [ ] 9.2 Ensure single-file download uses the `messageID` from the index entry
- [ ] 9.3 Write TDD tests for browse with index present and with fallback

## 10. GroupTotals — use the index

- [ ] 10.1 Update `GroupTotals` to sum file count and total size from each topic's index; for topics without an index, fall back to paginating that topic only
- [ ] 10.2 Write TDD tests for `GroupTotals` with mixed indexed/non-indexed topics

## 11. End-to-end and validation

- [ ] 11.1 Add an integration-style test covering the full migration: legacy topic → first sync builds index → second sync uses the fast path
- [ ] 11.2 Run the full test suite (`go test ./...`) and ensure it passes
- [ ] 11.3 Run `go vet`/lint and ensure no regressions
- [ ] 11.4 Validate the change with `openspec validate --change add-topic-index`
