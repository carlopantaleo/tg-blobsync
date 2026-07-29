## Why

Topic synchronization currently performs avoidable full-topic reads even when a valid index exists, including rebuilding the index after a sync and locating obsolete index messages. This increases Telegram API usage and latency. Chunked file handling also remains incomplete: pull progress counts physical chunks as separate files, and browse-based downloads cannot reconstruct chunked logical files.

## What Changes

- Prefer the existing topic index for synchronization and index maintenance whenever it is available.
- Update the index after synchronization using the known remote state and operation delta instead of rereading the entire topic.
- Avoid repeated full-topic reads, including repeated searches for obsolete index messages; introduce narrowly scoped caching or reuse of already fetched topic state where needed.
- Aggregate chunked files as one logical file for pull progress and report per-chunk progress within that file.
- Support downloading and reassembling chunked files selected through the browse flow.
- Preserve legacy full-topic scanning only when no usable index exists or migration is explicitly required.

## Capabilities

### New Capabilities

- `browse-chunked-download`: Download and reassemble chunked logical files selected from the browse interface.

### Modified Capabilities

- `topic-index`: Use the index as the primary remote state during sync and maintain it through deltas without unnecessary full-topic reads.
- `large-file-chunking`: Treat chunked files as one logical item in pull progress and browse downloads while exposing per-chunk progress.

## Impact

- **Use cases:** `internal/usecase/scanner.go`, `sync.go`, `browser.go`, executor/progress aggregation, and index migration/update flows.
- **Telegram adapter:** Index retrieval, topic listing/index message lookup, chunked download/reassembly, and related storage interfaces.
- **UI:** Progress reporting and browse download selection for logical chunked files.
- **Tests:** New and updated unit tests following a Red-Green-Refactor TDD cycle.
- **Compatibility:** No external API or storage format breaking changes are intended; indexed and legacy topics remain supported.
