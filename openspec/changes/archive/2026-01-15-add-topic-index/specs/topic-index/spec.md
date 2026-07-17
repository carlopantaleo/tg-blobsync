## ADDED Requirements

### Requirement: Topic index document
The system SHALL maintain, for each topic, a single INDEX message that is the most recent message in the topic and that contains a JSON document (`index.json`) with the complete metadata of every file stored in the topic. Each entry in the index SHALL include the file path, checksum, modification time, flags, size, and the Telegram message ID of the corresponding file message. The INDEX message caption SHALL be exactly `{"f":"INDEX"}` so that the index can be detected by inspecting only the last message of the topic.

#### Scenario: Index present and is the last message
- **WHEN** the most recent message of a topic has caption `{"f":"INDEX"}`
- **THEN** the system SHALL treat that message as the topic index and SHALL NOT treat it as a file

#### Scenario: Index entry contains size
- **WHEN** the system reads an entry from the index
- **THEN** the entry SHALL include a `size` field holding the file size in bytes

#### Scenario: Empty file represented in the index
- **WHEN** a file in the topic is a 0-byte file marked with the `EMPTY_FILE` flag
- **THEN** the corresponding index entry SHALL have `size` equal to 0 and `flags` equal to `EMPTY_FILE`

### Requirement: Remote state recovery from the index
The system SHALL recover the remote state of a topic from the index when the last message of the topic is the INDEX message, without paginating through all topic messages.

#### Scenario: Fast path recovery
- **WHEN** the last message of the topic is the INDEX message
- **THEN** the system SHALL download the `index.json` document and build the remote file map from its entries
- **AND** the system SHALL NOT paginate through the other messages of the topic

#### Scenario: Index used by browse
- **WHEN** the `browse` command lists files in a topic whose last message is the INDEX
- **THEN** the system SHALL build the browsable file list from the index
- **AND** single-file download SHALL use the `messageID` stored in the corresponding index entry

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
The system SHALL fall back to the legacy pagination flow when the last message of the topic is not the INDEX message. During the fallback, the system SHALL delete any stale INDEX messages found in the topic and SHALL create a fresh INDEX message as the last message of the topic before proceeding with synchronization.

#### Scenario: Legacy topic without index
- **WHEN** the last message of the topic is not the INDEX message
- **THEN** the system SHALL paginate through all topic messages to collect remote files
- **AND** the system SHALL delete every message whose caption is `{"f":"INDEX"}`
- **AND** the system SHALL upload a new INDEX message as the last message of the topic

#### Scenario: Stale index messages present during fallback
- **WHEN** the fallback pagination encounters one or more messages with caption `{"f":"INDEX"}`
- **THEN** the system SHALL delete each such message
- **AND** the system SHALL NOT include them in the remote file map

#### Scenario: Index message not treated as a file during fallback
- **WHEN** the fallback pagination encounters a message with caption `{"f":"INDEX"}`
- **THEN** the system SHALL NOT add it to the remote file map

### Requirement: Index rebuild after sync
The system SHALL rebuild the index after any synchronization that performs at least one change. The rebuild SHALL delete the previous INDEX message and upload a fresh INDEX message as the last message of the topic, reflecting the remote state after the sync.

#### Scenario: Sync with changes rebuilds the index
- **WHEN** a synchronization completes and at least one upload, download, update, or delete was performed
- **THEN** the system SHALL delete the previous INDEX message
- **AND** the system SHALL upload a new INDEX message as the last message of the topic reflecting the post-sync remote state

#### Scenario: Sync with no changes leaves the index untouched
- **WHEN** a synchronization completes and no changes were performed (everything up to date)
- **THEN** the system SHALL NOT delete or upload any INDEX message

### Requirement: Size handling without EMPTY_FILE special-case
The system SHALL populate `RemoteFile.Size` from the index entry's `size` field when the remote state is recovered from the index. The differ SHALL compare sizes directly without special-casing the `EMPTY_FILE` flag, because empty files are represented with `size` equal to 0 in the index.

#### Scenario: Skip-MD5 comparison uses index size
- **WHEN** the differ compares a local file against a remote file recovered from the index using modification time and size
- **THEN** the differ SHALL use the `size` from the index entry as the remote size
- **AND** the differ SHALL NOT apply any special-case adjustment for the `EMPTY_FILE` flag

#### Scenario: Legacy fallback normalizes empty file size
- **WHEN** the remote state is recovered via the legacy fallback and a file message has the `EMPTY_FILE` flag
- **THEN** the system SHALL set the `RemoteFile.Size` to 0
- **AND** the differ SHALL compare sizes directly without special-casing the `EMPTY_FILE` flag
