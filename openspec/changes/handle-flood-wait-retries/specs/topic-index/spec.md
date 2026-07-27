## MODIFIED Requirements

### Requirement: Remote state recovery from the index
The system SHALL recover the remote state of a topic from the index when the last message of the topic is the INDEX message, without paginating through all topic messages. For chunked entries, the recovered `RemoteFile` SHALL carry the `chunkIDs` list and the total logical size, and SHALL NOT be expanded into multiple files. If the Telegram history request used to inspect the latest topic message returns `FLOOD_WAIT (N)`, the system SHALL wait N seconds and retry the same request before returning an error.

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

### Requirement: Legacy fallback and automatic migration
The system SHALL fall back to the legacy pagination flow when the last message of the topic is not the INDEX message. During the fallback, the system SHALL delete any stale INDEX messages found in the topic and SHALL create a fresh INDEX message as the last message of the topic before proceeding with synchronization. The fallback SHALL group chunk messages into logical files and record their `chunkIDs` in the rebuilt index. Every history page request SHALL use the shared FLOOD_WAIT-aware retry policy.

#### Scenario: Legacy topic without index
- **WHEN** the last message of the topic is not the INDEX message
- **THEN** the system SHALL paginate through all topic messages to collect remote files
- **AND** the system SHALL group `CHUNK` messages into logical files with their `chunkIDs` sorted by `Idx`
- **AND** the system SHALL delete every message whose caption is `{"f":"INDEX"}`
- **AND** the system SHALL upload a new INDEX message as the last message of the topic

#### Scenario: Legacy history FLOOD_WAIT is retried
- **WHEN** a history page request returns `FLOOD_WAIT (N)`
- **THEN** the system SHALL wait N seconds
- **AND** the system SHALL retry the same page using the same pagination offset
- **AND** it SHALL not duplicate messages already collected from previous pages

#### Scenario: Stale index messages present during fallback
- **WHEN** the fallback pagination encounters one or more messages with caption `{"f":"INDEX"}`
- **THEN** the system SHALL delete each such message
- **AND** the system SHALL NOT include them in the remote file map

#### Scenario: Index message not treated as a file during fallback
- **WHEN** the fallback pagination encounters a message with caption `{"f":"INDEX"}`
- **THEN** the system SHALL NOT add it to the remote file map

### Requirement: Index rebuild after sync
The system SHALL rebuild the index after any synchronization that performs at least one change. The rebuild SHALL delete the previous INDEX message and upload a fresh INDEX message as the last message of the topic, reflecting the remote state after the sync, including `chunkIDs` for chunked logical files. Stale-index discovery SHALL retry FLOOD_WAIT responses using the server-provided duration.

#### Scenario: Sync with changes rebuilds the index
- **WHEN** a synchronization completes and at least one upload, download, update, or delete was performed
- **THEN** the system SHALL delete the previous INDEX message
- **AND** the system SHALL upload a new INDEX message as the last message of the topic reflecting the post-sync remote state, including `chunkIDs` for any chunked file

#### Scenario: Stale-index FLOOD_WAIT is retried
- **WHEN** stale-index discovery returns `FLOOD_WAIT (N)`
- **THEN** the system SHALL wait N seconds
- **AND** it SHALL retry stale-index discovery before failing the index rebuild

#### Scenario: Sync with no changes leaves the index untouched
- **WHEN** a synchronization completes and no changes were performed (everything up to date)
- **THEN** the system SHALL NOT delete or upload any INDEX message

## REMOVED Requirements

### Requirement: Takeout pagination for large topics
**Reason**: Normal Telegram history pagination is sufficient when each failed page waits for and retries the server-provided FLOOD_WAIT duration. Maintaining a second Takeout transport path duplicates pagination and cleanup logic.
**Migration**: Existing topics require no data migration. All future listing and stale-index discovery use normal `MessagesGetReplies` pagination with the shared retry policy.
