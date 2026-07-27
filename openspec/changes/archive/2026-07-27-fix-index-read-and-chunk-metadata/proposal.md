## Why

The current index reading mechanism is not centralized, causing inefficiencies during operations like `push` when interactively resolving the remote destination folder. This results in slow topic reads and occasional `FLOOD_WAIT` errors for large topics. Additionally, the chunked upload mechanism omits explicit index metadata (`"i": 0`) for the first chunk because it relies on 0-based indexing with `omitempty`, creating inconsistencies in metadata handling.

## What Changes

- Centralize index reading to ensure all operations, including the interactive remote folder resolution in `push`, utilize the index instead of performing full topic reads.
- Update the chunked upload logic to use 1-based indexing so that the first chunk explicitly includes `"i": 1` in its metadata, while non-chunked files naturally omit the field.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `topic-index`: Centralize the index read process to optimize interactive `push` operations and prevent `FLOOD_WAIT` errors.
- `large-file-chunking`: Change chunk indexing to be 1-based to ensure the first chunk always includes explicit index metadata.

## Impact

- `push` command and interactive remote folder resolution logic.
- Chunk upload metadata generation.
- Potential reduction in Telegram API calls during `push`.
