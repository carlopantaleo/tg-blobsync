## Context

Currently, in `internal/adapter/telegram/files.go`, the `UploadFile` function handles chunked file uploads. While the single-file upload portion utilizes a retry mechanism (`adapter.WithRetry`), the chunked portion does not wrap its loop operations with any retry logic. If an error occurs during a chunk upload, it immediately returns, failing the whole upload process. This is particularly problematic for large files where temporary network interruptions or rate limits are more common.

## Goals / Non-Goals

**Goals:**
- Make chunked uploads resilient to temporary network failures and API errors.
- Wrap chunk upload operations in a retry mechanism identical or equivalent to the one used for smaller files.

**Non-Goals:**
- Rewriting the entire file upload strategy or chunking algorithm.
- Modifying the retry policy parameters (we will use the existing configuration/defaults).

## Decisions

- **Wrap chunk upload with `WithRetry`**: We will wrap the inner function `b.client.UploadBigFile` inside `adapter.WithRetry`, similar to how `b.client.UploadFile` is wrapped for small files. 

## Risks / Trade-offs

- **Risk:** Increased upload time on failure.
  - Mitigation: The `WithRetry` has bounded retry attempts and backoffs, so it won't hang forever.
