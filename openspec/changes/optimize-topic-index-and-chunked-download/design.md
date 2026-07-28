## Context

The current architecture already has an indexed fast path in `scanner.ScanRemote`, but synchronization calls `rebuildIndex` after changes and that method lists the whole topic and then separately lists index messages. This duplicates Telegram history reads and rebuilds state from data that was already available before execution. The executor also operates on logical remote files, while progress and browse download behavior still need explicit chunk-aware handling.

## Goals / Non-Goals

**Goals:**
- Carry the indexed remote snapshot and synchronization delta through the push/pull flow so index updates do not require a full-topic reread.
- Reuse the known current index message ID and avoid a second history scan for stale index discovery when possible.
- Preserve a legacy fallback path for topics without a valid index.
- Aggregate chunked files as one progress item while reporting the active chunk and total chunks.
- Make browse downloads dispatch chunked `RemoteFile` entries through the same streaming reassembly path as pull downloads.

**Non-Goals:**
- Changing the index JSON schema or Telegram message format.
- Removing support for legacy topics without an index.
- Loading an entire chunked file into memory before writing it.
- Introducing a cross-run persistent cache whose invalidation could become stale.

## Decisions

- **Use an operation-scoped remote snapshot:** `ScanRemote` will expose enough state for synchronization to retain the indexed file map, index message ID, and whether the result came from the index. The executor will return the resulting remote file changes or identifiers needed to update the index. This is preferred over rereading Telegram because it keeps consistency within one sync and avoids network calls.
- **Apply index changes as a delta:** For indexed topics, update the in-memory `FileIndex` by removing deleted/replaced entries and adding uploaded entries, then upload one new index. Delete only the known previous index message. For legacy topics, retain migration/full scan behavior.
- **Cache only within one operation:** Reuse topic/index data during a single command invocation. Avoid a global cache because uploads, deletes, and concurrent commands make invalidation unsafe.
- **Centralize chunk-aware download:** Use one download path that accepts a logical `RemoteFile`; single files download by `MessageID`, while chunked files stream `ChunkIDs` in order. Browse will return the same logical download request used by pull.
- **Aggregate progress at the logical-file boundary:** The executor/UI will create one progress task per logical file and update its chunk position while each chunk is downloaded. Chunk completion must not increment the total file count.

## Risks / Trade-offs

- **[Risk]** Delta updates could omit an unexpected remote change made outside the synchronizer. → **Mitigation:** Use deltas only when the topic had a valid index and all operations succeeded; retain full rebuild for legacy/migration cases and preserve index metadata needed for validation.
- **[Risk]** Reusing a stale index message ID could delete the wrong message after concurrent changes. → **Mitigation:** Keep the current operation's index ID from `GetIndex`, and fall back to discovery/rebuild when the indexed precondition is not met.
- **[Risk]** Progress refactoring may affect existing single-file UI behavior. → **Mitigation:** Add focused tests for logical totals, chunk position, and existing single-file behavior before implementation.
- **[Risk]** Browse may expose malformed or incomplete chunk groups. → **Mitigation:** Validate `ChunkIDs` and return a descriptive download error without creating a partial destination.

## Migration Plan

No data migration is required. Existing indexed topics use the optimized path after deployment; legacy topics continue through the existing migration/fallback flow. Rollback consists of reverting the implementation commits, with no index format changes to undo.

## Open Questions

- Whether the executor should return a complete post-operation logical file map or a typed delta structure for index updates.
- Whether browse download destination naming needs a dedicated UI prompt for logical chunked files, or can reuse the existing request path unchanged.
