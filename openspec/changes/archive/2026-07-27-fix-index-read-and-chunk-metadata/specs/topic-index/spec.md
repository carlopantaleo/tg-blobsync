## MODIFIED Requirements

### Requirement: Remote state recovery from the index
The system SHALL recover the remote state of a topic from the index when the last message of the topic is the INDEX message, without paginating through all topic messages. For chunked entries, the recovered `RemoteFile` SHALL carry the `chunkIDs` list and the total logical size, and SHALL NOT be expanded into multiple files. If the Telegram history request used to inspect the latest topic message returns `FLOOD_WAIT (N)`, the system SHALL wait N seconds and retry the same request before returning an error. The system SHALL centralize index reading so that any operation that requires a remote file list (such as interactive path resolution during `push` or `pull`, `browse`, and `group totals`) uses the index if present, falling back to legacy pagination only if absent.

#### Scenario: Fast path recovery
- **WHEN** the last message of the topic is the INDEX message
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

#### Scenario: Index used by browse
- **WHEN** the `browse` command lists files in a topic whose last message is the INDEX
- **THEN** the system SHALL build the browsable file list from the index
- **AND** single-file download SHALL use the `messageID` stored in the corresponding index entry, or the `chunkIDs` list when the entry is chunked

#### Scenario: Browse prompts for index creation when absent
- **WHEN** the `browse` command lists files in a topic whose last message is not the INDEX
- **THEN** the system SHALL prompt the user asking whether to create the topic index
- **AND** if the user confirms, the system SHALL run the legacy fallback migration (paginate, delete stale indexes, upload new index) and then build the browsable file list from the new index
- **AND** if the user declines, the system SHALL fall back to the legacy `ListFiles` pagination without creating an index

#### Scenario: Index used by group totals
- **WHEN** `GroupTotals` is computed for a group
- **THEN** the system SHALL sum the file count and total size from the index of each topic that has one
- **AND** for any topic without an index the system SHALL fall back to paginating that topic's messages

#### Scenario: Group totals prompts for index creation when some topics lack an index
- **WHEN** `GroupTotals` is computed for a group and one or more topics do not have an index
- **THEN** the system SHALL prompt the user once asking whether to create indexes for all topics without one
- **AND** if the user confirms, the system SHALL run the legacy fallback migration for each topic without an index before computing totals
- **AND** if the user declines, the system SHALL compute totals for those topics using the legacy `ListFiles` pagination

#### Scenario: Index used by interactive path resolution
- **WHEN** the user is interactively prompted to select a remote sub-directory for `push` or `pull`
- **THEN** the system SHALL build the list of existing sub-directories from the topic index if present
- **AND** the system SHALL fall back to the legacy pagination without prompting if the index is absent
