## ADDED Requirements

### Requirement: Chunked file representation
The system SHALL represent a logical file larger than the configured chunk threshold as a sequence of Telegram document messages, each carrying a `FileMeta` caption with `Flags` equal to `CHUNK`, the logical file `Path`, `Checksum`, and `ModTime`, and an `Idx` field holding the 0-based position of the chunk within the logical file. The number of chunks SHALL be `ceil(size / chunkSize)` and the last chunk MAY be smaller than `chunkSize`.

#### Scenario: File above threshold is split
- **WHEN** a local file with size greater than `chunkThreshold` is uploaded
- **THEN** the system SHALL upload one Telegram document message per chunk
- **AND** each chunk message caption SHALL have `Flags` equal to `CHUNK` and a unique `Idx` starting at 0 and increasing by 1 per chunk
- **AND** every chunk message caption SHALL carry the same logical `Path`, `Checksum`, and `ModTime`

#### Scenario: File at or below threshold is not chunked
- **WHEN** a local file with size less than or equal to `chunkThreshold` is uploaded
- **THEN** the system SHALL upload exactly one Telegram document message with no `CHUNK` flag and no `Idx`

#### Scenario: Empty file is never chunked
- **WHEN** a 0-byte local file is uploaded
- **THEN** the system SHALL upload a single `EMPTY_FILE` message and SHALL NOT produce any chunk

### Requirement: Chunked file download and reassembly
The system SHALL download a chunked logical file by fetching its chunk messages in `Idx` order and concatenating their bytes into the destination file, exposing a single `io.ReadCloser` to the caller without buffering the whole file in memory.

#### Scenario: Download streams chunks in order
- **WHEN** the system downloads a chunked logical file
- **THEN** the system SHALL open chunk readers in increasing `Idx` order
- **AND** the returned reader SHALL yield the concatenation of the chunk bytes in order
- **AND** the system SHALL NOT load more than one chunk into memory at a time

#### Scenario: Missing chunk fails the download
- **WHEN** one of the expected chunk message IDs cannot be downloaded
- **THEN** the system SHALL return an error identifying the missing chunk index
- **AND** the system SHALL NOT write a partial file to the destination

### Requirement: Chunked file deletion
The system SHALL delete a chunked logical file by deleting every Telegram message whose ID appears in the logical file's `chunkIDs` list.

#### Scenario: Delete removes all chunks
- **WHEN** the system deletes a chunked logical file
- **THEN** the system SHALL delete every message ID listed in the file's `chunkIDs`
- **AND** the system SHALL NOT leave any orphan chunk message in the topic

### Requirement: Chunked file update
The system SHALL update a chunked logical file by uploading all new chunks first, then deleting every old chunk message listed in the previous `chunkIDs`.

#### Scenario: Update replaces all chunks
- **WHEN** a chunked logical file is updated
- **THEN** the system SHALL upload a fresh set of chunk messages
- **AND** after the new chunks are uploaded, the system SHALL delete every old chunk message from the previous `chunkIDs`
- **AND** the index SHALL be rebuilt with the new `chunkIDs`

### Requirement: Partial upload cleanup
The system SHALL make a best-effort attempt to delete any chunk messages already uploaded for a logical file when the upload of that file fails, so that no orphan chunks remain in the topic.

#### Scenario: Upload failure cleans up uploaded chunks
- **WHEN** uploading a chunked file fails after one or more chunks have been uploaded
- **THEN** the system SHALL delete the chunk messages uploaded during the failed attempt
- **AND** the failed file SHALL NOT be added to the topic index

### Requirement: Chunk threshold and size configuration
The system SHALL read `chunkThreshold` and `chunkSize` from configuration, with defaults of 2 GB and 1 GB respectively. A file SHALL be chunked if and only if its size is greater than `chunkThreshold`. The system SHALL validate that `chunkSize` is greater than 0 and not greater than `chunkThreshold`.

#### Scenario: Defaults apply when config is absent
- **WHEN** the configuration does not specify `chunkThreshold` or `chunkSize`
- **THEN** the system SHALL use 2 GB as the threshold and 1 GB as the chunk size

#### Scenario: Invalid chunk size is rejected
- **WHEN** `chunkSize` is configured to 0 or to a value greater than `chunkThreshold`
- **THEN** the system SHALL fail to start with a configuration validation error

### Requirement: Sync treats chunked file as single item
The differ and executor SHALL treat a chunked logical file as a single sync item keyed by its logical path, comparing the local file size against the remote logical total size, so that upload, download, update, and delete each operate on the whole logical file.

#### Scenario: Push diff for chunked file
- **WHEN** the differ computes a push plan and a local file larger than the threshold is new or changed
- **THEN** the plan SHALL contain exactly one sync item for that path with action `UPLOAD`
- **AND** the item summary `TotalSize` SHALL count the logical file size once

#### Scenario: Pull diff for chunked file
- **WHEN** the differ computes a pull plan and a remote chunked logical file is new or changed
- **THEN** the plan SHALL contain exactly one sync item for that path with action `DOWNLOAD`
- **AND** the item summary `TotalSize` SHALL count the logical file size once

### Requirement: Legacy fallback groups chunks into logical files
When the topic has no `INDEX` message, the legacy pagination fallback SHALL group messages with `Flags` equal to `CHUNK` into a single `RemoteFile` whose `ChunkIDs` are the chunk message IDs sorted by `Idx`. The grouping key SHALL be the logical `Path` together with `Checksum` when `Checksum` is non-empty; when `Checksum` is empty (e.g. the `--skip-md5` option is active), the grouping key SHALL fall back to `Path` alone, since all chunks of the same logical file share the same `Path` and `ModTime`. Non-chunked messages SHALL remain individual files.

#### Scenario: Fallback recovers chunked file as one entry
- **WHEN** the legacy fallback paginates a topic containing chunk messages for a logical file
- **THEN** the system SHALL emit a single `RemoteFile` for that logical file
- **AND** the `RemoteFile.ChunkIDs` SHALL list the chunk message IDs sorted by `Idx`
- **AND** the `RemoteFile.Size` SHALL equal the sum of the chunk document sizes

#### Scenario: Fallback groups chunks when checksum is absent
- **WHEN** the legacy fallback paginates a topic containing chunk messages whose `Checksum` is empty (sync run with `--skip-md5`)
- **THEN** the system SHALL group the chunk messages by `Path` alone
- **AND** the system SHALL emit a single `RemoteFile` per distinct `Path`

#### Scenario: Fallback ignores orphan chunk without matching set
- **WHEN** the legacy fallback encounters chunk messages that cannot be grouped into a complete ordered set (e.g. missing `Idx` 0)
- **THEN** the system SHALL skip those chunk messages and SHALL NOT emit a `RemoteFile` for them

### Requirement: Per-chunk progress reporting
The system SHALL report per-chunk progress for chunked uploads and downloads in addition to the aggregated logical-file progress. The progress task for a chunked file SHALL expose the current chunk index (1-based) and the total chunk count, so the UI can render a per-chunk indicator (e.g. `chunk 2/5`) alongside the overall byte progress.

#### Scenario: Upload reports per-chunk progress
- **WHEN** a chunked file is being uploaded
- **THEN** the progress task SHALL expose the current chunk index and total chunk count
- **AND** the byte progress SHALL aggregate across all chunks of the logical file

#### Scenario: Download reports per-chunk progress
- **WHEN** a chunked file is being downloaded
- **THEN** the progress task SHALL expose the current chunk index and total chunk count
- **AND** the byte progress SHALL aggregate across all chunks of the logical file
