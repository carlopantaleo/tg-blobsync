## Why

Telegram rate-limits history operations with `FLOOD_WAIT (N)`, but the current implementation returns the RPC error immediately. This makes large topics fail during stale-index discovery or pagination even though Telegram explicitly provides the required wait duration; a centralized, wait-aware retry policy can make these operations reliable and remove the unnecessary Takeout-specific code path.

## What Changes

- Detect Telegram `FLOOD_WAIT` errors and wait for the server-provided number of seconds before retrying the same operation.
- Make the existing retry helper FLOOD_WAIT-aware while preserving context cancellation and bounded retry behavior.
- Apply the retry policy consistently to topic history, latest-message lookup, stale-index discovery, upload/download setup, and other Telegram RPC operations currently exposed to rate limits.
- Remove the `ListFiles` Takeout API branch, Takeout session initialization/finalization, and message-count threshold logic; use normal `MessagesGetReplies` pagination with wait-aware retries instead.
- Update tests and documentation to cover rate-limit recovery and the simplified pagination flow.

## Capabilities

### New Capabilities
- `flood-wait-recovery`: Reliable retry and delay behavior for Telegram `FLOOD_WAIT` responses.

### Modified Capabilities
- `topic-index`: Stale-index discovery and latest-index lookup SHALL recover from `FLOOD_WAIT` instead of failing immediately.

## Impact

- **Retry package** (`internal/pkg/retry`): parse Telegram RPC errors, wait the indicated duration, and retry within the configured limit.
- **Telegram adapter** (`internal/adapter/telegram`): remove Takeout-specific listing logic and wrap history/index RPC calls with the shared retry helper.
- **Tests**: retry timing/cancellation tests, Telegram pagination tests, and removal of Takeout-only assumptions.
- **User-visible behavior**: large topics may pause while Telegram's requested wait elapses, then continue automatically; persistent rate limiting still returns an error after bounded retries.
- **Dependencies**: no new dependencies.
