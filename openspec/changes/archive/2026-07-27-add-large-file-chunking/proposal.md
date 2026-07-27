## Why

Telegram caps single document uploads at 2 GB (4 GB for premium accounts). Files larger than that cannot be synced today: `UploadFile` fails and the whole sync aborts. Users with large media, backups, or archives have no way to store them through tg-blobsync, which defeats the goal of using Telegram as a general-purpose blob store.

## What Changes

- Introduce a `CHUNK` flag and an `idx` metadata field on `FileMeta` so that a single logical file can be represented by multiple Telegram messages (one per chunk).
- Split local files larger than a configurable threshold (default 2 GB) into ordered chunks before upload; each chunk is uploaded as its own document message with `CHUNK` flag and increasing `idx`.
- Reassemble chunked files at download by reading all chunk messages for a logical file in `idx` order and concatenating their bytes into the destination file.
- Track chunked files in the topic index: a single `FileIndexEntry` per logical file, carrying the total size and the list of chunk message IDs (new `chunkIDs` field) so the remote state can be recovered without paginating.
- Extend sync (differ/executor) to treat a chunked logical file as a single sync item: upload re-uploads all chunks (and deletes the old chunk messages on update), download fetches and concatenates all chunks, delete removes every chunk message.
- **BREAKING**: `FileMeta` and `FileIndexEntry` gain new optional JSON fields (`idx`, `chunkIDs`). Older clients that ignore unknown fields keep working for non-chunked files, but cannot download chunked files.

## Capabilities

### New Capabilities
- `large-file-chunking`: Splitting, storing, and reassembling files larger than Telegram's single-message limit by representing one logical file as multiple ordered `CHUNK` messages, with index support and sync integration.

### Modified Capabilities
- `topic-index`: The index document gains a `chunkIDs` field on entries to record the message IDs of every chunk of a chunked logical file, and the recovery/migration flows SHALL preserve and rebuild chunk metadata.

## Impact

- **Domain** (`internal/domain`): `FileMeta` (add `Idx`), `FileIndexEntry` (add `ChunkIDs`), `RemoteFile` (carry chunk IDs), new chunking constants and helpers.
- **Telegram adapter** (`internal/adapter/telegram`): `UploadFile`/`DownloadFile`/`DeleteFile`/`ListFiles`/`parseMessageToFile` updated to handle chunked files; new chunk split/assemble helpers.
- **Index** (`internal/adapter/telegram/index.go`): serialize/parse `chunkIDs`; legacy fallback must group chunk messages into a single logical file.
- **Use cases** (`internal/usecase`): `differ` compares logical files (size = total size); `executor` upload/download/delete handle multi-message chunked items.
- **Config** (`internal/config`): new `chunkThreshold` and `chunkSize` options (defaults 2 GB and 1 GB).
- **Tests**: TDD coverage for chunk split/assemble, index round-trip with `chunkIDs`, differ/executor behavior for chunked files.
- **Dependencies**: none new (uses existing `gotd` uploader/downloader and stdlib `io`/`os`).
