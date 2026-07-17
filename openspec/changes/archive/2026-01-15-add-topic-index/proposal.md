## Why

The current sync engine rebuilds the remote state by paginating through every message in a topic and parsing each caption. For topics with many files this is slow and consumes Telegram API quota (with a fallback to the Takeout API above 3000 messages). Persisting a single index document containing all file metadata lets the engine recover the remote state in one download, dramatically reducing sync time and API calls.

## What Changes

- Introduce an **INDEX** message stored as the most recent message in a topic, containing a JSON document (`index.json`) with the full metadata of every file in the topic (path, checksum, modTime, flags, size, messageID).
- The INDEX message caption carries a minimal `{"f":"INDEX"}` marker so it can be detected by fetching only the last message of the topic.
- When the last message is the INDEX, the engine reads the remote state from it instead of paginating all messages.
- When the last message is not the INDEX (legacy topic, missing or stale index), the engine falls back to the legacy pagination flow, deletes any stale INDEX messages found, rebuilds the INDEX as the last message, and then proceeds with the sync.
- After every sync that performs changes, the engine deletes the previous INDEX message and uploads a fresh one as the last message, reflecting the new remote state.
- When a sync results in no changes (everything up to date), the INDEX is left untouched.
- The `browse` command and `GroupTotals` aggregation are updated to leverage the INDEX where applicable, avoiding full pagination.
- The `size` field is now persisted in the index, removing the need for the special-case `EMPTY_FILE` size handling in the differ when the index is available.

## Capabilities

### New Capabilities
- `topic-index`: Persist and maintain a per-topic index document containing the complete metadata of all files in the topic, used as the primary source of remote state for sync, browse, and totals.

### Modified Capabilities
<!-- No existing specs to modify. -->

## Impact

- **Domain**: New `FileIndex`/`FileIndexEntry` entities and a new `INDEX` flag value. `RemoteFile` already carries `Size`; the index entries embed it plus `MessageID`.
- `BlobStorage` interface: new operations to fetch the last message, detect the INDEX, download and upload the index document, and delete a message by ID (already present).
- `TelegramClient` adapter: implement index detection, download, upload, and the legacy fallback with stale-index cleanup.
- `Scanner`: `ScanRemote` reads from the index when available instead of calling the legacy `ListFiles`.
- `Synchronizer` (`Push`/`Pull`): after a successful sync with changes, rebuild the INDEX (delete old, upload new as last message).
- `Browser` and `GroupTotals`: use the index path.
- `Differ`: simplify `EMPTY_FILE` size handling when size is sourced from the index.
- Tests: TDD coverage for index detection, fallback migration, rebuild, and no-op skip.
