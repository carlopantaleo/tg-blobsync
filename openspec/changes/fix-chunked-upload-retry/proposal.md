## Why

When uploading a chunked file in the `UploadFile` function, the program currently exits immediately upon encountering an error instead of applying retry logic. This behavior is inconsistent with single-file uploads where a `WithRetry` mechanism is employed, resulting in reduced robustness and failed uploads for larger files.

## What Changes

- Add a `WithRetry` mechanism around the upload logic for chunked files in `internal/adapter/telegram/files.go`.
- Ensure that network failures or temporary errors during chunked uploads trigger retries instead of immediately returning or exiting.

## Capabilities

### New Capabilities

- `chunked-upload-retry`: Adding resilience and retry policies for chunked file uploads in the Telegram adapter.

### Modified Capabilities

- None

## Impact

- **Affected code:** `internal/adapter/telegram/files.go` (specifically the chunked upload block within `UploadFile`).
- **Impact:** Improved stability and resilience for handling large file uploads. No breaking changes or external API changes.
