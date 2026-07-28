## 1. Setup

- [x] 1.1 Create and checkout fix branch `fix/optimize-topic-index-and-chunked-download`
- [x] 1.2 Commit milestone: Set up topic index and chunked download fixes

## 2. Indexed Synchronization Delta (Red-Green-Refactor)

- [x] 2.1 Write failing tests proving indexed synchronization does not call full-topic listing during post-sync index update and reuses the known index message ID.
- [x] 2.2 Write failing tests for applying upload, update, and delete operations to the retained index snapshot.
- [x] 2.3 Run the focused tests and verify they fail (Red).
- [x] 2.4 Implement an operation-scoped indexed remote snapshot and typed synchronization delta for index maintenance.
- [x] 2.5 Update synchronization/index rebuild logic to apply deltas for indexed topics and preserve full-scan fallback for legacy topics.
- [x] 2.6 Ensure stale-index discovery and duplicate topic reads remain bounded and errors are propagated correctly.
- [x] 2.7 Run focused tests and verify they pass (Green).
- [x] 2.8 Refactor the index update flow for clear ownership and minimal repeated data fetching.
- [x] 2.9 Run the focused tests again after refactoring.
- [x] 2.10 Commit milestone: Optimize indexed synchronization reads

## 3. Chunked Pull Progress (Red-Green-Refactor)

- [x] 3.1 Write failing tests proving a chunked pull contributes one logical file to totals and exposes current chunk/total chunk progress.
- [x] 3.2 Write failing tests proving non-chunked pull progress remains unchanged.
- [x] 3.3 Run the focused tests and verify they fail (Red).
- [x] 3.4 Implement logical-file progress aggregation in the executor and UI progress reporting for chunked downloads.
- [x] 3.5 Ensure chunk completion does not increment the completed-file counter and errors identify the affected chunk.
- [x] 3.6 Run focused tests and verify they pass (Green).
- [x] 3.7 Refactor progress handling to share logical-file behavior between single-file and chunked downloads.
- [x] 3.8 Run the focused tests again after refactoring.
- [x] 3.9 Commit milestone: Fix chunked pull progress aggregation

## 4. Browse Chunked Downloads (Red-Green-Refactor)

- [x] 4.1 Write failing tests proving browse lists an indexed chunked entry once and preserves ordered chunk IDs.
- [x] 4.2 Write failing tests proving browse download streams chunked files into one destination and rejects incomplete chunks.
- [x] 4.3 Run the focused tests and verify they fail (Red).
- [x] 4.4 Implement browse download dispatch through the logical `RemoteFile` download/reassembly path.
- [x] 4.5 Ensure indexed and legacy browse results use the same chunk-aware download behavior without loading the whole file into memory.
- [x] 4.6 Run focused tests and verify they pass (Green).
- [x] 4.7 Refactor browse and download flow to avoid duplicate chunk handling.
- [x] 4.8 Run the focused tests again after refactoring.
- [x] 4.9 Commit milestone: Support browse downloads for chunked files

## 5. Verification

- [x] 5.1 Run the complete Go test suite and resolve regressions.
- [x] 5.2 Run formatting and static checks used by the repository.
- [x] 5.3 Review the final diff against the proposal, design, and specs.
- [x] 5.4 Commit milestone: Verify index and chunked download fixes
