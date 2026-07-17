## Context

The current sync engine recovers the remote state of a topic by paginating through every message via `MessagesGetReplies` (or the Takeout API above 3000 messages) and parsing each message caption as a `FileMeta`. The `size` of each file is read from the Telegram `Document` media, not from the metadata. This is correct but slow and API-quota-hungry for large topics.

The new design introduces a single **INDEX** message per topic that contains a JSON document with the complete metadata of all files in the topic. The engine reads the remote state from this single document when present, falling back to the legacy flow otherwise.

### Current data model

```
FileMeta  { Path, Checksum, ModTime, Flags }      // stored in message caption
RemoteFile { Meta, MessageID, Size }              // Size read from Document media
```

`Flags` currently uses `EMPTY_FILE` to mark 0-byte files (Telegram rejects 0-byte uploads; a 1-byte dummy is uploaded). The differ special-cases `EMPTY_FILE` to treat `remoteSize` as 0 when `--skip-md5` is used.

### Proposed data model

```
FileIndexEntry { Path, Checksum, ModTime, Flags, Size, MessageID }   // one per file
FileIndex      { Entries []FileIndexEntry }                          // the index document
```

The INDEX message:

- **caption**: `{"f":"INDEX"}` (a `FileMeta` with only the `Flags` field set to `INDEX`). This is the minimal detectable marker.
- **attachment**: `index.json` — the serialized `FileIndex`.

A new flag value `INDEX` is introduced alongside `EMPTY_FILE`. `parseMessageToFile` must skip INDEX messages when collecting file entries (they are not files).

## Goals / Non-Goals

**Goals:**
- Recover remote state in O(1) message fetches + 1 download when the index is present.
- Provide an automatic, transparent migration path from legacy topics (no manual rebuild step required by the user).
- Keep the index consistent at the end of every sync that performs changes.
- Reuse the index for `browse` and `GroupTotals` to avoid full pagination.
- Persist `size` in the index so the differ no longer needs the `EMPTY_FILE` size special-case when the index is the source.

**Non-Goals:**
- Concurrent multi-client consistency. The project assumes a single active client per topic; lost-update on the index between concurrent clients is accepted and self-heals on the next legacy fallback (which rediscovers orphan file messages).
- Indexing across topics. Each topic maintains its own index.
- Compression or delta encoding of the index. Plain JSON is sufficient for the expected scale.
- Replacing the legacy `ListFiles` pagination entirely. It is retained as the fallback/migration path.

## Decisions

### D1: Index stored as a document attachment, not in the caption
Telegram captions are limited to ~1024 characters. An index for hundreds/thousands of files cannot fit. The index is therefore uploaded as a `index.json` document, with the caption carrying only the `{"f":"INDEX"}` marker.

**Alternative considered:** index in caption — rejected due to the size limit.

### D2: Detection by fetching only the last message
`MessagesGetReplies` with `offsetID=0` and `limit=1` returns the most recent message in the topic. If its caption parses to a `FileMeta` with `Flags == "INDEX"`, the engine uses the fast path. Otherwise it falls back to the legacy flow.

**Alternative considered:** scanning the first page for an INDEX marker — rejected; the index is always the last message by construction, so a single fetch is sufficient and cheaper.

### D3: Fallback flow performs migration
When the last message is not an INDEX:
1. Run the legacy `ListFiles` pagination to collect remote files.
2. While paginating, collect any message whose caption is `{"f":"INDEX"}` (stale/dirty indexes) and delete them.
3. Build a `FileIndex` from the collected remote files and upload it as the last message of the topic.
4. Proceed with diff + execute using the legacy-collected state.

This guarantees that after the first sync of a legacy topic, an index always exists.

### D4: Post-sync rebuild — delete old, then upload new (option B)
After a sync that performed at least one change:
1. Delete the previous INDEX message (if known).
2. Build a fresh `FileIndex` reflecting the new remote state and upload it as the last message.

Order: delete-then-upload (option B). If a crash occurs between the two steps, the index is lost; the next sync detects the missing index and runs the fallback migration, which rebuilds it. This is acceptable given the rarity of crashes and the self-healing fallback.

**Alternative considered:** upload-new-then-delete-old (option A) — more robust (always at least one index present) but leaves two indexes temporarily, requiring the fallback to handle "multiple stale indexes" (which it already does via step 2 of D3). Rejected to keep the implementation simpler; the fallback already covers the crash case.

### D5: Skip rebuild on no-op syncs
If `plan.Summary.Total == 0` (everything up to date), the index is still valid and is not rewritten. This avoids unnecessary API calls and message churn.

### D6: Index contains all files in the topic, not just the current subDir
The index is per-topic and contains every file's metadata regardless of which subDir a particular sync targets. The `scanner` keeps responsibility for filtering by `subDir` client-side, exactly as it does today with the legacy `ListFiles` result. This keeps the index reusable across multiple subDir syncs on the same topic.

### D7: `size` in the index removes the `EMPTY_FILE` size special-case
When the remote state is sourced from the index, `RemoteFile.Size` is populated from `FileIndexEntry.Size`. For `EMPTY_FILE` entries the stored size is 0, so `shouldUpdate` with `--skip-md5` no longer needs to special-case the flag. The special-case is retained only for the legacy fallback path (where `Size` comes from the `Document` media and is 1 for empty files). To keep behavior uniform, the legacy path will normalize `Size` to 0 when `Flags == "EMPTY_FILE"` when building `RemoteFile`, so the differ special-case can be removed entirely.

### D8: `browse` and `GroupTotals` use the index
- `browse`: read the index; each entry already carries `MessageID`, so single-file download works unchanged.
- `GroupTotals`: aggregate per-topic indexes across the group's topics (sum `Files` count and `TotalSize`). If a topic has no index, fall back to the legacy per-topic pagination for that topic only.

### D9: New `BlobStorage` operations
Add to the `BlobStorage` interface:
- `GetIndex(ctx, groupID, topicID) (*FileIndex, int, bool, error)` — returns the index, the INDEX message ID, and whether an index was present. (Fetches last message, detects marker, downloads `index.json`.)
- `UploadIndex(ctx, groupID, topicID, index FileIndex) (int, error)` — uploads `index.json` as the last message and returns the new message ID.
- `DeleteIndex(ctx, groupID, topicID, messageID int) error` — reuses the existing `DeleteFile` (delete by message ID).

The legacy `ListFiles` is retained for the fallback path and is extended to also return any detected INDEX message IDs so the fallback can clean them up.

## Risks / Trade-offs

- **[Crash between delete and upload of the index]** → The index is lost. Mitigation: the next sync runs the fallback migration, which rebuilds the index from the legacy pagination. Self-healing. Accepted (D4).
- **[Concurrent clients overwrite each other's index]** → Files uploaded by one client may not appear in the other client's index, becoming "orphans" until a fallback rebuild. Mitigation: out of scope; single-client assumption documented. A fallback rebuild rediscovers orphans.
- **[Index grows large for very big topics]** → Download/parse latency increases. Mitigation: plain JSON is compact; for realistic scales (thousands of files) the index is a few hundred KB. Not a concern for the target use case.
- **[Stale index after out-of-band edits]** → If files are added/removed in the topic by means other than this tool, the index drifts. Mitigation: the fallback path (triggered when the last message is not an index, e.g. because a non-index message was appended) rebuilds the index. Manual fallback can be forced by deleting the index message.
- **[Migration cost on first sync of a legacy topic]** → The first sync pays the legacy pagination cost once, plus an index upload. Subsequent synces are fast. Acceptable one-time cost.
- **[Backward compatibility]** → Older clients that do not understand the `INDEX` flag will treat the index message as a non-file (caption does not match the `Path != "" && (Checksum != "" || ModTime != 0)` predicate) and skip it. They will not benefit from the index but will not break. Newer clients reading a topic written by an older client fall back to legacy pagination and migrate. No breaking change.

## Migration Plan

1. Implement the index read/write and the fallback migration.
2. On the first sync of any existing topic, the fallback path runs automatically and creates the index. No user action required.
3. The legacy `ListFiles` path remains as the fallback; no data format change to existing file messages.
4. Rollback: delete the `INDEX` message from a topic to force the fallback on the next sync (which would recreate it). To fully revert, disable the new code path; old clients ignore the index message.

## Open Questions

None. All key decisions resolved during exploration.
