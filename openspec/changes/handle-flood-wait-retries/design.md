## Context

Telegram returns an RPC error such as `FLOOD_WAIT (25)` when a client exceeds a rate limit. The current `internal/pkg/retry` helper retries only with a fixed exponential delay and does not inspect the server-provided wait duration. Several Telegram history operations are not wrapped at all. In addition, `TelegramClient.ListFiles` switches to `account.initTakeoutSession` after a message-count threshold, which creates a second pagination path and still does not protect the initial history request from `FLOOD_WAIT`.

The relevant operations are normal topic history pagination in `files.go`, stale INDEX discovery in `index.go`, latest-topic-message lookup in `index.go`, and existing upload/download setup operations already using `retry.WithRetry`.

## Goals / Non-Goals

**Goals:**
- Detect `FLOOD_WAIT` errors and wait at least the server-provided duration before retrying.
- Keep retry behavior cancellable through `context.Context` and bounded by the existing retry limit.
- Apply the policy to every Telegram history/index request that can be rate-limited.
- Remove Takeout initialization, invocation, finalization, and the message-count threshold from topic file listing.
- Preserve pagination ordering, termination behavior, and error wrapping for non-rate-limit failures.

**Non-Goals:**
- Retrying arbitrary permanent Telegram RPC errors indefinitely.
- Changing the number of pagination items requested per call.
- Concurrentizing history requests or introducing a global rate limiter.
- Retrying after context cancellation or deadline expiration.
- Changing Telegram upload/download retry counts or progress behavior except where they use the shared FLOOD_WAIT-aware helper.

## Decisions

### Decision 1: Parse FLOOD_WAIT from the RPC error text
Implement a small parser in `internal/pkg/retry` that recognizes the canonical `FLOOD_WAIT (N)` form and returns `N` seconds. It SHALL also tolerate common wrappers around the RPC message (for example `rpc error code 420: FLOOD_WAIT (25)`). Non-matching errors return `(0, false)`.

**Why:** The gotd error type is wrapped by several call paths and the stable, documented signal available at this boundary is the RPC error text/code. Keeping parsing in the retry package avoids Telegram-specific parsing scattered across adapters.

**Alternatives considered:**
- Depend on a concrete gotd RPC error type: rejected because wrapped errors and version-specific types make the adapter brittle.
- Use only exponential backoff: rejected because it can retry before Telegram's mandatory wait expires.

### Decision 2: FLOOD_WAIT delay takes precedence over exponential backoff
On a failed attempt with a parsed wait duration, `WithRetry` SHALL wait `N` seconds (optionally plus a small safety margin) before the next attempt, instead of applying the normal exponential delay. The wait SHALL be interruptible by the context. The helper SHALL not sleep after the final failed attempt.

**Why:** Telegram's duration is authoritative and avoids repeated failures caused by retrying too early. The existing exponential backoff remains the fallback for transient errors without a server-provided duration.

### Decision 3: Centralize retry at the operation boundary
Wrap each `MessagesGetReplies` call in `ListFiles`, `ListIndexMessageIDs`, and `getLatestTopicMessage` with `retry.WithRetry`. Do not retry the whole pagination loop: a retry repeats only the failed page with the same `offsetID`, preventing duplicate accumulation and preserving progress.

Existing upload/download setup calls continue to use the same helper and automatically gain FLOOD_WAIT handling.

### Decision 4: Remove Takeout entirely from ListFiles
`ListFiles` SHALL always use `MessagesGetReplies` with the existing offset/limit pagination. Remove `useTakeout`, `takeoutID`, the deferred `AccountFinishTakeoutSession`, `InvokeWithTakeoutRequest`, `AccountInitTakeoutSessionRequest`, and the `Count > 3000` branch. The result parsing SHALL continue supporting all normal Telegram response variants.

**Why:** Wait-aware retries make normal pagination robust without maintaining two transport paths. This reduces state, cleanup, and mock complexity while retaining the same remote file behavior.

### Decision 5: Preserve bounded retries and context semantics
The existing `maxRetries` argument remains authoritative. A `FLOOD_WAIT` on the last attempt is returned immediately; context cancellation during the wait returns `ctx.Err()`. The final error retains the operation name and original error as its cause.

## Risks / Trade-offs

- **[A long server wait blocks a sync]** → Mitigation: this is required by Telegram; the wait is logged, cancellable, and bounded by the existing retry count.
- **[The error text format changes]** → Mitigation: parse the canonical `FLOOD_WAIT` token and numeric seconds defensively; non-matching errors keep existing retry behavior.
- **[Normal pagination may still hit Telegram limits]** → Mitigation: each page is retried in place; the caller can cancel, and bounded retries prevent an infinite loop.
- **[Removing Takeout may alter behavior for very large topics]** → Mitigation: preserve the current page size and offset termination logic; add regression tests for multi-page responses and explicitly verify no Takeout requests occur.
- **[Repeated side effects if future callers wrap non-idempotent operations]** → Mitigation: document that retry operations must be safe to repeat; current history/index operations are read-only and uploads already generate fresh IDs per attempt.

## Migration Plan

1. Add parser and retry behavior with unit tests while preserving the current helper signature.
2. Wrap history/index page requests and remove Takeout code/tests.
3. Run the full Go test suite and `go vet ./...`.
4. Deploy normally; existing topics and index formats are unchanged.
5. Rollback by reverting the binary if needed; no persisted data migration is required.

## Open Questions

- Should a safety margin be added to the server-provided wait (for example, one second), or should the exact duration be used? The initial implementation should use the exact duration unless tests or production observations demonstrate boundary timing issues.
