## MODIFIED Requirements

### Requirement: Remote state recovery from the index
The system SHALL recover the remote state of a topic from the index when the last message of the topic is the INDEX message, without paginating through all topic messages. For chunked entries, the recovered `RemoteFile` SHALL carry the `chunkIDs` list and the total logical size, and SHALL NOT be expanded into multiple files. If the Telegram history request used to inspect the latest topic message returns `FLOOD_WAIT (N)`, the system SHALL wait N seconds and retry the same request before returning an error. The system SHALL centralize index reading so that any operation that requires a remote file list (such as interactive path resolution during `push` or `pull`, `browse`, and `group totals) uses the index if present, falling back to legacy pagination only if absent. During a synchronization that starts from a valid index, the system SHALL retain the indexed remote snapshot and SHALL NOT reread the entire topic merely to reconstruct the post-sync index.

#### Scenario: Fast path recovery
- **WHEN** the last message of a topic is the INDEX message
- **THEN** the system SHALL download the `index.json` document and build the remote file map from its entries
- **AND** the system SHALL NOT paginate through the other messages of the topic

#### Scenario: Fast path recovery preserves chunked entries
- **WHEN** the index contains an entry with `flags` equal to `CHUNK` and a non-empty `chunkIDs`
- **THEN** the recovered remote file map SHALL contain a single `RemoteFile` for that entry
- **AND** the `RemoteFile.ChunkIDs` SHALL equal the entry's `chunkIDs` in order
- **AND** the `RemoteFile.Size` SHALL equal the entry's total logical size

#### Scenario: Latest-message FLOOD_WAIT is retried
- **WHEN** the request that checks the latest topic message returns `FLOOD_WAIT (N)`
- **THEN** the system SHALL wait N seconds
- **AND** the system SHALL retry the same latest-message request
- **AND** it SHALL continue index recovery if the retry succeeds

#### Scenario: Persistent latest-message rate limit fails boundedly
- **WHEN** every allowed retry of the latest-message request returns FLOOD_WAIT
- **THEN** the system SHALL return an error after the configured retry limit
- **AND** it SHALL preserve the underlying Telegram error

#### Scenario: Indexed synchronization reuses the remote snapshot
- **WHEN** a synchronization starts from a valid topic index and performs changes
- **THEN** the system SHALL update the retained indexed snapshot using the synchronization delta
- **AND** the system SHALL NOT paginate through the entire topic to rebuild the index

### Requirement: Index rebuild after sync
The system SHALL update the index after any synchronization that performs at least one change. When synchronization starts from a valid index, the update SHALL apply the known upload, update, and delete delta to the retained remote snapshot, delete the known previous INDEX message, and upload a fresh INDEX message as the last message of the topic. When synchronization starts from a legacy topic, the system SHALL retain the existing full rebuild behavior. The resulting index SHALL reflect the remote state after the sync, including `chunkIDs` for chunked logical files. Stale-index discovery SHALL retry FLOOD_WAIT responses using the server-provided duration only when the previous index cannot be identified from the operation-scoped state.

#### Scenario: Indexed sync updates by delta
- **WHEN** a synchronization completes and at least one change was performed on a topic with a valid index
- **THEN** the system SHALL apply the operation delta to the retained index entries
- **AND** the system SHALL delete the previous known INDEX message
- **AND** the system SHALL upload one new INDEX message as the last message of the topic

#### Scenario: Legacy sync rebuilds from a full scan
- **WHEN** a synchronization completes and the topic did not have a valid index at scan time
- **THEN** the system SHALL use the legacy full-topic state to create the new index
- **AND** the system SHALL preserve the existing stale-index cleanup behavior

#### Scenario: Sync with no changes leaves the index untouched
- **WHEN** a synchronization completes and no changes were performed (everything up to date)
- **THEN** the system SHALL NOT delete or upload any INDEX message
