## Context

The system currently reads the topic index in operations like `browse` and `group totals`, but interactive path resolution (during `push` or `pull`) bypasses the index and invokes legacy pagination directly via `ListFiles`. This causes slow execution and `FLOOD_WAIT` errors on large topics.
Additionally, chunked files omit the `"i": 0` metadata from the first chunk's caption due to the `omitempty` struct tag on the `Idx` field in `domain.FileMeta`.

## Goals / Non-Goals

**Goals:**
- Optimize interactive sub-directory selection by attempting to use the topic index first.
- Ensure all chunks of a file explicitly specify their index in the JSON metadata by switching to 1-based indexing.

**Non-Goals:**
- Prompting for index creation during `push`/`pull` interactive sub-directory resolution (we will silently fall back to legacy pagination to maintain CLI flow).
- Refactoring the entire `identifierResolver` interface beyond what is necessary.

## Decisions

**Index Read Centralization:**
- In `cmd/tgblobsync/main.go`'s `resolveIdentifiersInternal`, before invoking `storage.ListFiles(ctx, groupID, topicID)` to build the subdirectories map, the system will first attempt `blobStorage.GetIndex`.
- If the index is present (`indexed == true`), it will use `index.RemoteFiles()` instead of `ListFiles`.
- If absent, it will fall back to `storage.ListFiles`.

**Explicit Chunk Index Metadata (1-based indexing):**
- **Decision**: Change the chunk index (`Idx`) semantics to start from 1 instead of 0. Because `Idx` will be non-zero for the first chunk (1), the `omitempty` tag will naturally include it in the JSON output. For non-chunked files, `Idx` will remain 0 (the default), and `omitempty` will omit it.
- **Alternative considered**: Implement a custom `MarshalJSON` on `domain.FileMeta`. *Discarded* because it adds unnecessary complexity to the serialization layer and requires careful alias typing to avoid recursion.
- **Alternative considered**: Remove `omitempty` from `Idx`. *Discarded* because it would emit `"i": 0` on every single non-chunked file, bloating the topic index and caption payloads unnecessarily.
- **Alternative considered**: Change `Idx` to `*int`. *Discarded* because it introduces unnecessary pointer indirections and nil checks.

## Risks / Trade-offs

- **Risk**: Existing 0-based chunk indexes in the wild will need to be handled carefully if there are any backwards-compatibility concerns.
  - **Mitigation**: Since this is still an active development phase, migrating to 1-based indexing is acceptable. The `chunkIDs` slice handles ordering natively during recovery regardless of the underlying index value, as long as it sorts properly.
