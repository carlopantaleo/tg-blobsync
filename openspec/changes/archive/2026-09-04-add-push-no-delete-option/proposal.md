## Why

Both `push` and `pull` currently always perform destructive deletions of files that exist only on one side: `push` deletes remote files missing locally (`DELETE_REMOTE`), and `pull` deletes local files missing remotely (`DELETE_LOCAL`). Users need a safe, additive-only sync mode that transfers new/changed files without removing anything on either side.

## What Changes

- Add a `--no-delete` boolean flag to the `push` and `pull` commands of the CLI.
- When `--no-delete` is set on `push`, the sync plan must exclude all `DELETE_REMOTE` operations: remote files absent locally are kept untouched.
- When `--no-delete` is set on `pull`, the sync plan must exclude all `DELETE_LOCAL` operations: local files absent remotely are kept untouched.
- When the flag is omitted, current deletion behavior on both commands remains unchanged.
- Sync summary/reporting must reflect that deletions were skipped (no `To Delete` count when the flag is active).

## Capabilities

### New Capabilities
- `sync-deletion-control`: Controls whether `push` and `pull` delete files missing on the opposite side, via an opt-in `--no-delete` flag that produces an additive-only sync in both directions.

### Modified Capabilities
<!-- No existing spec covers sync deletion behavior; this is a new capability. -->

## Impact

- `internal/config/cli.go`: new `NoDelete` field in `CLIConfig` and `--no-delete` flag registration; documented under `push` and `pull` usage.
- `internal/usecase/differ.go`: `DiffPush` and `DiffPull` accept a deletion-control option and skip emitting `ActionDeleteRemote` / `ActionDeleteLocal` items respectively when active.
- `internal/usecase/sync.go`: `Synchronizer` wires the option from config into the differ for both `Push` and `Pull`.
- `cmd/tgblobsync/main.go`: pass `cfg.NoDelete` to the synchronizer for both sync paths.
- Tests: new/updated unit tests for CLI parsing, differ behavior (both directions), and sync wiring (TDD).
- Documentation in `doc/` if a CLI reference exists.
