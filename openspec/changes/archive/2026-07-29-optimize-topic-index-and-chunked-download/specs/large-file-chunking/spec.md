## MODIFIED Requirements

### Requirement: Chunked file download and reassembly
The system SHALL download a chunked logical file by fetching its chunk messages in `Idx` order and concatenating their bytes into the destination file, exposing a single `io.ReadCloser` to the caller without buffering the whole file in memory. The download operation SHALL expose one logical-file progress task and SHALL report the current 1-based chunk index and total chunk count without counting each chunk as a separate file.

#### Scenario: Download streams chunks in order
- **WHEN** the system downloads a chunked logical file
- **THEN** the system SHALL open chunk readers in increasing `Idx` order
- **AND** the returned reader SHALL yield the concatenation of the chunk bytes in order
- **AND** the system SHALL NOT load more than one chunk into memory at a time

#### Scenario: Missing chunk fails the download
- **WHEN** one of the expected chunk message IDs cannot be downloaded
- **THEN** the system SHALL return an error identifying the missing chunk index
- **AND** the system SHALL NOT write a partial file to the destination

#### Scenario: Pull reports one logical file for chunks
- **WHEN** a chunked logical file is downloaded during pull
- **THEN** the synchronization progress total SHALL count one file for the logical file
- **AND** progress SHALL expose the active chunk index and total chunk count
- **AND** completed chunks SHALL NOT increment the completed-file count individually

### Requirement: Sync treats chunked file as single item
The differ and executor SHALL treat a chunked logical file as a single sync item keyed by its logical path, comparing the local file size against the remote logical total size, so that upload, download, update, and delete each operate on the whole logical file. Progress summaries SHALL use the logical item count and logical total size rather than the number of physical chunks.

#### Scenario: Push diff for chunked file
- **WHEN** the differ computes a push plan and a local file larger than the threshold is new or changed
- **THEN** the plan SHALL contain exactly one sync item for that path with action `UPLOAD`
- **AND** the item summary `TotalSize` SHALL count the logical file size once

#### Scenario: Pull diff for chunked file
- **WHEN** the differ computes a pull plan and a remote chunked logical file is new or changed
- **THEN** the plan SHALL contain exactly one sync item for that path with action `DOWNLOAD`
- **AND** the item summary `TotalSize` SHALL count the logical file size once
- **AND** execution progress SHALL count one file, not one file per chunk

### Requirement: Per-chunk progress reporting
The system SHALL report per-chunk progress for chunked uploads and downloads in addition to the aggregated logical-file progress. The progress task for a chunked file SHALL expose the current chunk index (1-based) and the total chunk count, so the UI can render a per-chunk indicator (e.g. `chunk 2/5`) alongside the overall byte progress. The logical file SHALL be the only item contributing to the synchronization file totals.

#### Scenario: Upload reports per-chunk progress
- **WHEN** a chunked file is being uploaded
- **THEN** the progress task SHALL expose the current chunk index and total chunk count
- **AND** the byte progress SHALL aggregate across all chunks of the logical file

#### Scenario: Download reports per-chunk progress
- **WHEN** a chunked file is being downloaded
- **THEN** the progress task SHALL expose the current chunk index and total chunk count
- **AND** the byte progress SHALL aggregate across all chunks of the logical file
- **AND** the file counter SHALL advance once after the logical file is complete
