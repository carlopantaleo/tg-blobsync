## Context

Synchronization flows through: CLI parsing (`internal/config/cli.go`) → `main.go` `runSync` → `usecase.Synchronizer.Push`/`Pull` → `usecase.NewDiffer(skipMD5).DiffPush`/`DiffPull` → `usecase.Executor.Execute`. `DiffPush` emits `ActionDeleteRemote` items for remote files missing locally; `DiffPull` emits `ActionDeleteLocal` items for local files missing remotely; the executor applies them. There is no way to suppress deletions in either direction. See proposal.md for motivation.

## Goals / Non-Goals

**Goals:**
- Opt-in `--no-delete` flag for both `push` and `pull`, skipping remote deletions on push and local deletions on pull, with minimal code churn.
- Behavior change isolated at the plan level (differ), so the executor and index-update logic remain untouched.

**Non-Goals:**
- No config-file/persistent setting; CLI flag only.
- No changes to executor, index update, or upload/download paths.
- No distinction between push and pull for the flag name: a single `--no-delete` that means "delete nothing on the target side of the sync".

## Decisions

- **Filter deletions in the differ, not in the executor.** The plan is the source of truth for what will happen: if `ActionDeleteRemote`/`ActionDeleteLocal` items are never emitted, the summary (`To Delete: 0`), the confirmation prompt, and the execution are all consistent automatically. Alternative considered: skipping deletes in `executor.Execute` — rejected because the summary/confirmation would still announce deletions that won't occur, misleading the user.
- **Plumb the option as a boolean on `Differ`.** `NewDiffer(skipMD5)` gains the flag (e.g., `NewDiffer(skipMD5, noDelete)`) or a setter, matching existing style; `Synchronizer` gets `SetNoDelete(bool)` called from `main.go` for both push and pull. Alternative considered: an options struct — rejected as unnecessary for a single boolean (YAGNI) and inconsistent with the current constructor style.
- **One shared flag for both commands.** The user's intent ("do not delete anything") is symmetric; a single `--no-delete` keeps the CLI surface small and predictable. Alternative considered: separate `--no-delete-remote`/`--no-delete-local` — rejected as redundant granularity for the stated use case.
- **Flag naming:** `--no-delete` (explicit, mirrors user request). Default `false` preserves current behavior (no breaking change).
- **Index consistency is automatic:** with a delta index update, opposite-side-only files stay in the retained index because no `Deleted` entries are produced; no extra work needed.

## Risks / Trade-offs

- [User expects --no-delete to also protect against update-replacement deletes] → Clarify in help text/docs: updating a changed file still replaces its old remote version (upload + delete old message on push); `--no-delete` only suppresses deletions of files not present on the opposite side. This distinction is inherent to how updates work and applies unchanged today.
- [The global `flag.FlagSet` registers the flag for all commands, including `browse`] → Acceptable: browse performs no deletions; behavior is wired into push/pull paths only and help is documented under those usages.
