## 1. CLI flag parsing

- [x] 1.1 Create and checkout branch `feature/add-push-no-delete-option`
- [ ] 1.2 Add failing tests in `internal/config/cli.go`'s test file (e.g. `internal/config/config_test.go`) asserting `--no-delete` sets `CLIConfig.NoDelete=true` for both `push` and `pull`, and defaults to `false`
- [ ] 1.3 Implement: add `NoDelete bool` field to `CLIConfig` and register `fs.BoolVar(&cfg.NoDelete, "no-delete", false, ...)` in `internal/config/cli.go`, and document it in the `push` and `pull` usage help
- [ ] 1.4 Run the new config tests until green; commit: `feat(cli): add --no-delete flag for push and pull`

## 2. Differ skips deletions in both directions

- [ ] 2.1 Add failing tests in `internal/usecase/differ_test.go`: with no-delete active, `DiffPush` emits no `ActionDeleteRemote` items and `DiffPull` emits no `ActionDeleteLocal` items, while upload/update/download items are still emitted; without the option both diffs are unchanged
- [ ] 2.2 Implement: extend `NewDiffer` in `internal/usecase/differ.go` to accept the no-delete option (constructor arg or setter, per design); skip `ActionDeleteRemote` emission in `DiffPush` and `ActionDeleteLocal` emission in `DiffPull` when active; update existing `NewDiffer` call sites
- [ ] 2.3 Run differ tests until green; commit: `feat(usecase): skip deletions in differ when no-delete is set`

## 3. Sync wiring and end-to-end behavior

- [ ] 3.1 Add failing tests for `Synchronizer`: `Push` with no-delete set produces a plan with `Summary.ToDelete == 0` and no remote deletions reach the storage mock; `Pull` with no-delete set produces `Summary.ToDelete == 0` and no local deletions reach the filesystem mock
- [ ] 3.2 Implement: add `SetNoDelete(bool)` (or equivalent) on `Synchronizer` in `internal/usecase/sync.go`, pass it to `NewDiffer` in both `Push` and `Pull`; wire `cfg.NoDelete` in `cmd/tgblobsync/main.go` `runSync` for both commands
- [ ] 3.3 Verify full test suite (`go test ./...`) and `go build ./...` pass
- [ ] 3.4 Update user documentation in `doc/` if a CLI/command reference doc exists (mention `--no-delete` on push and pull)
- [ ] 3.5 Commit: `feat(sync): wire --no-delete from CLI to synchronizer`
