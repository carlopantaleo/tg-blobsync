## Purpose

Controls whether `push` and `pull` delete files that exist only on one side of the synchronization, allowing users to run an additive-only sync in both directions that never removes existing content.

## ADDED Requirements

### Requirement: Push no-delete flag

The CLI SHALL accept a `--no-delete` boolean flag on the `push` command. When provided, the push synchronization SHALL NOT delete any remote file, even if it is absent from the local directory. When the flag is omitted, the current behavior (deleting remote files missing locally) SHALL remain unchanged.

#### Scenario: Push with --no-delete keeps remote-only files

- **GIVEN** a remote topic containing file `old.txt` that does not exist locally
- **WHEN** the user runs `push --no-delete`
- **THEN** the sync plan contains no delete operations for `old.txt`
- **AND** after sync completes, `old.txt` still exists on the remote topic

#### Scenario: Push without --no-delete keeps current behavior

- **GIVEN** a remote topic containing file `old.txt` that does not exist locally
- **WHEN** the user runs `push` without `--no-delete`
- **THEN** the sync plan contains a delete operation for `old.txt`
- **AND** after sync completes, `old.txt` is removed from the remote topic

#### Scenario: --no-delete still uploads new and changed files

- **GIVEN** a local file `new.txt` that does not exist remotely and a local file `changed.txt` whose content differs from the remote version
- **WHEN** the user runs `push --no-delete`
- **THEN** `new.txt` is uploaded and `changed.txt` is updated on the remote topic

### Requirement: Pull no-delete flag

The CLI SHALL accept the `--no-delete` boolean flag on the `pull` command. When provided, the pull synchronization SHALL NOT delete any local file, even if it is absent from the remote topic. When the flag is omitted, the current behavior (deleting local files missing remotely) SHALL remain unchanged.

#### Scenario: Pull with --no-delete keeps local-only files

- **GIVEN** a local file `local-only.txt` that does not exist in the remote topic
- **WHEN** the user runs `pull --no-delete`
- **THEN** the sync plan contains no delete operations for `local-only.txt`
- **AND** after sync completes, `local-only.txt` still exists locally

#### Scenario: Pull without --no-delete keeps current behavior

- **GIVEN** a local file `local-only.txt` that does not exist in the remote topic
- **WHEN** the user runs `pull` without `--no-delete`
- **THEN** the sync plan contains a delete operation for `local-only.txt`
- **AND** after sync completes, `local-only.txt` is removed locally

### Requirement: Sync summary consistency with --no-delete

When `--no-delete` is active, the sync summary and confirmation prompt SHALL report zero deletions (`To Delete: 0`) and SHALL NOT list any files to delete, so the user is never asked to confirm deletions that will not occur.

#### Scenario: Summary shows no deletions with --no-delete

- **GIVEN** files exist on one side that are missing on the other
- **WHEN** the user runs `push --no-delete` or `pull --no-delete`
- **THEN** the summary reports `To Delete: 0`
- **AND** no file is listed as a pending deletion
