## Purpose

Lets users running `push` or `pull` interactively navigate into nested remote directories and pick a subdirectory at any depth, instead of being limited to first-level folders or typing nested paths by hand.

## ADDED Requirements

### Requirement: Drill-down subdirectory navigation

When the user selects a topic interactively for `push` or `pull` and remote files exist, the subdirectory selection MUST present a hierarchical navigation: at each level the UI shows the current directory path, a "[ This directory ]" entry to confirm the current level, an "[ Enter custom path ]" entry, the immediate (single-level) subdirectories of the current level derived from remote file paths, and an entry to go back up one level.

#### Scenario: Navigating into a nested directory

- **GIVEN** a topic containing remote files under `a/b/c/`
- **WHEN** the user reaches the subdirectory selection and selects `a`, then `b`
- **THEN** the menu shows current path `a/b` and offers "This directory", "Enter custom path", and the child directory `c`

#### Scenario: Confirming the root directory

- **GIVEN** a topic with remote files in subdirectories
- **WHEN** the user selects "This directory" at the root level (empty current path)
- **THEN** the sync is scoped to the whole topic (no subdirectory), as today

#### Scenario: Going back up

- **GIVEN** the user is inside subdirectory `a/b`
- **WHEN** the user selects the up/back entry
- **THEN** the menu returns to level `a`
- **AND** selecting up/back at the root level returns to topic selection, as today

#### Scenario: Directory with no subdirectories

- **GIVEN** the user is inside a directory whose remote files have no deeper subdirectories
- **WHEN** the menu is displayed
- **THEN** only "This directory", "Enter custom path", and the up/back entry are shown

### Requirement: Selected deep subdirectory scopes the sync

When the user confirms a directory at any depth via "This directory", the synchronization scope MUST be that full path, applying the same relative-path filtering and prefixing behavior already used for first-level selections.

#### Scenario: Push limited to selected deep subdir

- **GIVEN** a topic with remote files under `a/b/` and under `other/`
- **WHEN** the user drills down and confirms `a/b` as the subdirectory, then runs `push`
- **THEN** remote comparison and subsequent uploads/deletions consider only paths under `a/b`
- **AND** uploaded files are stored under the `a/b/` remote prefix

### Requirement: Custom path relative to current directory

The "Enter custom path" entry MUST interpret the typed path as relative to the current navigation directory, not to the topic root, and confirm the resulting combined path.

#### Scenario: Custom path under current directory

- **GIVEN** the user is inside subdirectory `a/b`
- **WHEN** the user selects "Enter custom path" and types `x/y`
- **THEN** the sync is scoped to `a/b/x/y`

#### Scenario: Custom path at root unchanged

- **GIVEN** the user is at the root level
- **WHEN** the user selects "Enter custom path" and types `x/y`
- **THEN** the sync is scoped to `x/y`, matching today's behavior
