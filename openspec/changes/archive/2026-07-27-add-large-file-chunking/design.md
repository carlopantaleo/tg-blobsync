## Context

tg-blobsync stores each local file as a single Telegram document message in a forum topic, with a JSON `FileMeta` caption (`p`, `m`, `t`, `f`). The topic index (`INDEX` message) keeps one `FileIndexEntry` per logical file (`p`, `m`, `t`, `f`, `s`, `id`). Upload uses `gotd`'s `uploader.FromPath`, download streams via `downloader.Download(...).Stream(...)`, and the differ/executor treat each file as a single message identified by `MessageID`.

Telegram rejects documents larger than 2 GB (4 GB on premium). Today any file above the limit fails the whole sync. The change introduces logical files composed of multiple physical chunk messages, while keeping the existing single-message fast path for files under the threshold.

## Goals / Non-Goals

**Goals:**
- Sync files larger than 2 GB by splitting them into ordered chunk messages.
- Preserve the existing single-message behavior for files under the threshold (no overhead, no regression).
- Keep the topic index as the single source of truth for remote state, including chunked files.
- Treat a chunked logical file as one sync item (upload/download/delete) so the differ and executor stay simple.
- TDD coverage for split/assemble, index round-trip, and sync flows.

**Non-Goals:**
- Parallel chunk upload (keep sequential for v1; can be added later without API change).
- Resumable/partial chunk upload across process restarts.
- Reusing existing chunks on update (an update re-uploads all chunks and deletes the old ones).
- Premium-aware 4 GB threshold (v1 uses a conservative 2 GB default; configurable).

## Decisions

### Decision 1: Chunk metadata in caption, not in a sidecar
Each chunk message carries a `FileMeta` with the logical `Path`, `Checksum`, `ModTime`, `Flags="CHUNK"`, and a new `Idx` field (0-based chunk index). The logical file's identity (path + checksum) is shared by all chunks; `Idx` orders them.

**Why:** Reuses the existing caption parsing (`parseMessageToFile`) with one extra field. No new message types. The index message already serializes `FileMeta`-derived fields, so adding `Idx` is consistent.

**Alternatives considered:**
- A separate `CHUNK_INDEX` message listing chunk IDs: rejected, adds a second control message to maintain and migrate.
- Storing order in the document filename: rejected, fragile and not queryable.

### Decision 2: One `FileIndexEntry` per logical file, with `chunkIDs`
The index entry for a chunked file stores the total logical `size` and a new `chunkIDs []int` field holding the message IDs of every chunk in `Idx` order. `MessageID` is kept for backward compatibility but is unused for chunked entries (set to the first chunk's ID).

**Why:** The index stays one-entry-per-logical-file, so the differ and totals keep working unchanged. `chunkIDs` lets download/delete operate without paginating the topic.

**Alternatives considered:**
- One index entry per chunk: rejected, inflates the index and breaks the "one entry = one file" invariant used by totals/browse.
- Storing only the first chunk ID and walking replies: rejected, slow and racy.

### Decision 3: Threshold and chunk size are configurable
New config fields `chunkThreshold` (default 2 GB) and `chunkSize` (default 1 GB). A file is chunked iff `size > chunkThreshold`. Number of chunks = `ceil(size / chunkSize)`. The last chunk may be smaller.

**Why:** Conservative defaults respect Telegram's 2 GB limit for non-premium accounts while keeping chunk count low. Configurability lets premium users raise the threshold and tune chunk size for their network.

### Decision 4: Chunking lives in the Telegram adapter, logical view in domain
`domain` exposes `RemoteFile.ChunkIDs []int` and `FileMeta.Idx int`. The Telegram adapter owns the split/assemble logic (it knows the transport limit). The use cases see a `RemoteFile`/`LocalFile` and call `UploadFile`/`DownloadFile`/`DeleteFile` as today; the adapter decides whether to chunk based on `LocalFile.Size` vs `chunkThreshold`.

**Why:** Keeps the domain transport-agnostic and concentrates the new complexity where the constraint exists (SOLID: the constraint owner owns the workaround).

### Decision 5: Upload = upload all chunks then delete old chunks on update
`UploadFile` for a chunked file: split, upload each chunk as its own message (sequential, with progress aggregated across chunks), collect the new message IDs. On update (`executor` sees `item.RemoteFile != nil`), the executor deletes every old `ChunkIDs` message after the new chunks are uploaded, mirroring the current single-message update flow.

**Why:** Atomicity is best-effort (Telegram has no transaction), but uploading new chunks before deleting old ones minimizes the window where the file is missing.

### Decision 6: Download = stream chunks in order through an `io.MultiReader`
`DownloadFile` for a chunked file opens chunk readers lazily in `Idx` order and exposes a single `io.ReadCloser` that concatenates them. We avoid buffering whole chunks in memory by chaining readers: when one chunk reader is exhausted, the next is opened.

**Why:** Memory-bounded, integrates with the existing `fs.WriteFile(path, reader)` streaming path.

### Decision 7: Legacy fallback groups chunks by `Path` (and `Checksum` when present)
When a topic has no `INDEX` message, `ListFiles` paginates and groups messages with `Flags=="CHUNK"` into a single `RemoteFile` with `ChunkIDs` sorted by `Idx`. The grouping key is the logical `Path` plus `Checksum` **when `Checksum` is non-empty**; when `Checksum` is empty (e.g. the `--skip-md5` option is in use), the grouping key falls back to `Path` alone, since all chunks of the same logical file share the same `Path` and `ModTime`. The first chunk encountered provides `MessageID`. Non-chunked messages stay single files. The rebuilt index then stores `chunkIDs`.

**Why:** `Checksum` is an optional field (`json:"m,omitempty"`) and is not populated when `--skip-md5` is active, so it cannot be assumed as a stable grouping key in every configuration. `Path` is always present and unique per logical file within a topic, making it a reliable primary key; `Checksum` is used only as an additional disambiguator when available. Keeps the migration path consistent with the existing "paginate, delete stale INDEX, upload new INDEX" fallback.

## Risks / Trade-offs

- **[Partial upload failure leaves orphan chunks]** → Mitigation: on upload error, best-effort delete the chunks already uploaded in the current attempt; the file is not added to the index, so the next sync retries the whole file.
- **[Index grows with chunkIDs for many large files]** → Mitigation: `chunkIDs` is a slice of ints; for typical use (a few large files) this is negligible. If it becomes a problem, a future change can store a chunk manifest document instead.
- **[Old clients cannot download chunked files]** → Mitigation: documented as BREAKING in the proposal; `chunkIDs` is optional JSON so old clients still parse non-chunked entries.
- **[Sequential chunk upload is slower than parallel]** → Mitigation: accepted for v1 (Non-Goal); the uploader already uses concurrent parts internally for each chunk.
- **[Telegram message count per topic grows with chunks]** → Mitigation: accepted; the index keeps recovery O(1) regardless of message count.
- **[Chunk size vs. premium 4 GB limit]** → Mitigation: defaults target the 2 GB non-premium limit; premium users can raise `chunkThreshold`/`chunkSize` via config.

## Migration Plan

1. Ship domain + adapter changes behind the new config fields (defaults preserve current behavior: files ≤ 2 GB keep using the single-message path).
2. Existing topics with non-chunked files keep working unchanged; the index gains an optional `chunkIDs` field that old entries simply omit.
3. The first time a file larger than the threshold is uploaded, the adapter creates chunk messages and the post-sync index rebuild records `chunkIDs`.
4. Rollback: revert the binary; chunked files become undownloadable by the old binary (documented BREAKING), but non-chunked files are unaffected. To fully roll back, delete chunked entries from the index and remove their chunk messages manually.

## Open Questions

- Should `chunkSize` be capped to the configured Telegram limit at startup? Probably yes (validate `chunkSize <= chunkThreshold <= 2GB` unless explicitly overridden).

## Resolved Decisions

- **Per-chunk progress in the TUI**: yes, the progress UI SHALL surface per-chunk progress (e.g. `chunk 2/5`) for chunked uploads/downloads, while still aggregating the overall logical-file progress. The `ProgressTask` for a chunked file SHALL expose the current chunk index and total chunk count in addition to the byte-level progress, so the TUI can render both views.
