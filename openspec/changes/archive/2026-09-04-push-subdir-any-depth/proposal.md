## Why

In interactive mode, `push` (and `pull`) currently offer only first-level subdirectories of a topic for selection: in `cmd/tgblobsync/main.go` `resolveIdentifiersInternal`, the list of existing subdirectories is built from `parts[0]` of each remote file path. Users targeting a nested folder like `a/b/c` must type it manually via "Enter custom path", which is error-prone and undiscoverable.

## What Changes

- The interactive subdirectory selection becomes a hierarchical drill-down navigation instead of a flat first-level list.
- At each level the user sees: "[ This directory ]" (confirm the current level), "[ Enter custom path ]" (relative to the current directory, no longer to the topic root), the immediate subdirectories of the current level, and an entry to go back up (at the root level this keeps the current "back to topics" behavior).
- Selecting a subdirectory enters it and repeats the same menu one level deeper; selecting a subdirectory at any depth scopes the sync to that path.
- No change to CLI target parsing: `<group>:<topic>:<subdir>` already accepts multi-segment subdir values.
- Applies to both `push` and `pull` since both share the same interactive selection flow; no behavioral change to non-interactive usage.

## Capabilities

### New Capabilities
- `subdir-selection`: Interactive drill-down selection of a remote subdirectory at any nesting depth when running `push` or `pull` interactively.

### Modified Capabilities
<!-- No existing spec covers subdirectory selection; this is a new capability. -->

## Impact

- `cmd/tgblobsync/main.go`: `resolveIdentifiersInternal` subdirectory selection becomes a navigation loop over immediate children of the current prefix, instead of collecting first-level names once.
- `internal/adapter/ui/console.go`: `SelectSubDir` evolves to render the current path plus drill-down entries (this directory / custom path / child directories / up); the `selectionUI` interface in `main.go` changes accordingly.
- New helper to compute immediate child directory names of a prefix from the remote file list (pure function, unit-testable).
- No impact on scanner/executor: they already normalize and handle multi-segment `subDir` values; `SetSubDir(cfg.SubDir)` behavior is unchanged.
