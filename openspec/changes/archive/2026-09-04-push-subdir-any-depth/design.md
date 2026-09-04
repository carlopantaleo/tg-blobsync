## Context

Interactive subdirectory selection lives in `cmd/tgblobsync/main.go` `resolveIdentifiersInternal` (SubDir selection block): it collects first-level directory names from remote file paths and passes them once to `console.SelectSubDir(existing []string)`, which renders a flat list with fixed entries ("back", "root", "custom path"). Downstream (`usecase.scanner` and `executor`) already normalize and correctly handle multi-segment `subDir` values, and `parseTarget` already passes multi-segment subdir values from the CLI. See proposal.md for motivation.

## Goals / Non-Goals

**Goals:**
- Hierarchical drill-down navigation over existing remote directories, following the menu structure requested by the user: "This directory", "Enter custom path" (relative to current dir), immediate subdirectories, up/back.

**Non-Goals:**
- No changes to scanner/executor/subDir semantics or CLI argument parsing.
- No flattened any-depth list (explicitly rejected by user feedback).
- No fetch of additional remote data: navigation derives from the remote file list/index already loaded at this step.

## Decisions

- **Navigation loop in `main.go`, UI kept dumb.** `resolveIdentifiersInternal` runs a loop holding the current prefix; each iteration computes immediate child directories and calls a revised `SelectSubDir`. The UI renders items and returns a semantic result; state (current path) lives in the loop. Alternative considered: stateful UI handling the whole navigation — rejected, as it would push path logic into the presentation layer and complicate headless testing.
- **Result encoding via a small struct or sentinel returns.** `SelectSubDir(existingSubDirs []string, currentPath string)` returns either a selected child name (enter it), a sentinel for "this directory", "custom" (prompt, interpreted relative to current path), "up", or "back" (at root, propagates today's topic-back flow). A dedicated result type is preferred over overloading string sentinels if it fits the existing `selectionUI` interface style; otherwise keep string returns consistent with current `"back"`/`""`/`"custom"` conventions.
- **Immediate-children helper as a pure function.** `immediateSubDirs(files []domain.RemoteFile, prefix string) []string` strips `prefix+"/"` from matching paths and takes the first remaining segment; dedup via map, sorted. Kept in `main.go`'s package (like current inline logic) or extracted for testability per tasks.
- **Custom path combination:** typed input is joined with the non-empty current prefix (`prefix + "/" + input`) and normalized like today; at root it behaves exactly as current behavior.
- **No subdirectories at current level:** skip listing child entries; if the topic has no subdirectories at all, preserve today's behavior of showing only root/custom/back entries.

## Risks / Trade-offs

- [Deep navigation adds UI round trips vs. typing a full path] → Mitigated by keeping "Enter custom path" available at every level, relative to the current directory.
- [Sentinel/result protocol between main.go and console.go becomes more complex than today's] → Kept minimal (enter/up/this/custom) and covered by headless UI tests like the existing `SelectSubDir` test in `internal/adapter/ui/console_test.go`.
- [Directories containing no files (empty dirs) are not discoverable] → Same limitation as today; reachable via custom path.
