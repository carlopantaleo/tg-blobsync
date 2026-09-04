## 1. Immediate-children helper

- [ ] 1.1 Create and checkout branch `feature/push-subdir-any-depth`
- [ ] 1.2 Add failing tests in `cmd/tgblobsync/main_test.go` for a new pure helper `immediateSubDirs(files []domain.RemoteFile, prefix string) []string`: nested paths yield only immediate child segments under the given prefix, deduplicated and sorted; prefix with no deeper dirs yields an empty list; non-matching prefixes ignored
- [ ] 1.3 Implement `immediateSubDirs` in `cmd/tgblobsync/main.go` and run tests until green; commit: `feat(cli): add helper to list immediate child directories`

## 2. Drill-down navigation UI

- [ ] 2.1 Add failing headless tests in `internal/adapter/ui/console_test.go` for the revised `SelectSubDir`: it renders current path, "[ This directory ]", "[ Enter custom path ]", immediate children, and up/back; verify each selection maps to the agreed result (enter child / this / custom / up / back)
- [ ] 2.2 Implement in `internal/adapter/ui/console.go` the revised `SelectSubDir(existingSubDirs []string, currentPath string)` (signature per design) with the new menu; update the `selectionUI` interface in `cmd/tgblobsync/main.go`
- [ ] 2.3 Run UI tests until green; commit: `feat(ui): add drill-down subdirectory selection menu`

## 3. Navigation loop wiring

- [ ] 3.1 Add failing tests around `resolveIdentifiersInternal` (or an extracted navigation function) in `cmd/tgblobsync/main_test.go` using fake resolver/UI: entering `a` then `b` then "This directory" sets `cfg.SubDir` to `a/b`; "custom" input `x/y` inside `a` sets `cfg.SubDir` to `a/x/y`; up/back at root preserves the existing "back to topics" error flow
- [ ] 3.2 Implement the navigation loop in the SubDir selection block of `resolveIdentifiersInternal` in `cmd/tgblobsync/main.go`: compute children per level via `immediateSubDirs`, handle enter/up/this/custom results, combine custom paths with the current prefix
- [ ] 3.3 Run tests until green; commit: `feat(cli): navigate subdirectories at any depth when selecting push/pull scope`

## 4. Verification

- [ ] 4.1 Run full test suite (`go test ./...`) and `go build ./...`
- [ ] 4.2 Update user documentation in `doc/` if a CLI/command reference doc exists (describe drill-down subdirectory selection)
- [ ] 4.3 Commit: `docs: document drill-down subdirectory selection`
