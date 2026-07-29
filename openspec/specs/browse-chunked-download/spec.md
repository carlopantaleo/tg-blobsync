# Browse Chunked Download Specification

## Purpose

Defines how the browse flow presents and downloads chunked logical files as single downloadable items.

## Requirements

### Requirement: Browse downloads chunked logical files
The browse flow SHALL present chunked entries from the topic index or legacy logical-file grouping as a single downloadable file and SHALL reconstruct the selected file by streaming its chunks in order.

#### Scenario: Browse lists a chunked file once
- **WHEN** browse loads a topic containing an indexed chunked logical file
- **THEN** the file list SHALL contain exactly one entry for the logical path
- **AND** the entry SHALL retain the ordered `chunkIDs` needed for download

#### Scenario: Browse downloads chunks in order
- **WHEN** the user selects a chunked file for download
- **THEN** the browse flow SHALL issue a logical `DownloadRequest` containing the chunked remote file
- **AND** the download implementation SHALL stream chunks in `Idx` order into one destination file
- **AND** the user SHALL receive one completed-file result rather than one result per chunk

#### Scenario: Browse rejects an incomplete chunked file
- **WHEN** a selected chunked entry has a missing or invalid chunk message
- **THEN** the download SHALL fail with an error identifying the affected chunk
- **AND** the destination SHALL not be reported as successfully downloaded
