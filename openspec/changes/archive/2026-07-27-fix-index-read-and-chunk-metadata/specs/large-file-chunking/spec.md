## MODIFIED Requirements

### Requirement: Chunked file representation
The system SHALL represent a logical file larger than the configured chunk threshold as a sequence of Telegram document messages, each carrying a `FileMeta` caption with `Flags` equal to `CHUNK`, the logical file `Path`, `Checksum`, and `ModTime`, and an `Idx` field holding the 1-based position of the chunk within the logical file. The `Idx` field SHALL be explicitly included in the serialized JSON metadata for all chunks, starting with `Idx=1` for the first chunk. The number of chunks SHALL be `ceil(size / chunkSize)` and the last chunk MAY be smaller than `chunkSize`.

#### Scenario: File above threshold is split
- **WHEN** a local file with size greater than `chunkThreshold` is uploaded
- **THEN** the system SHALL upload one Telegram document message per chunk
- **AND** each chunk message caption SHALL have `Flags` equal to `CHUNK` and a unique `Idx` starting at 1 and increasing by 1 per chunk
- **AND** every chunk message caption SHALL carry the same logical `Path`, `Checksum`, and `ModTime`

#### Scenario: File at or below threshold is not chunked
- **WHEN** a local file with size less than or equal to `chunkThreshold` is uploaded
- **THEN** the system SHALL upload exactly one Telegram document message with no `CHUNK` flag and no `Idx`

#### Scenario: Empty file is never chunked
- **WHEN** a 0-byte local file is uploaded
- **THEN** the system SHALL upload a single `EMPTY_FILE` message and SHALL NOT produce any chunk

#### Scenario: First chunk explicitly serializes its index
- **WHEN** the system serializes the metadata for the first chunk of a chunked file
- **THEN** the JSON output SHALL explicitly contain `"i": 1` and SHALL NOT omit the field
