## 1. Retry parser and FLOOD_WAIT behavior (TDD)

- [x] 1.1 Add a parser in `internal/pkg/retry` for canonical and wrapped `FLOOD_WAIT (N)` errors, returning seconds and a match flag
- [x] 1.2 Add RED tests for canonical, wrapped, malformed, and unrelated errors
- [x] 1.3 Update `WithRetry` to use the parsed server duration in preference to exponential backoff
- [x] 1.4 Add tests proving the next attempt is delayed by the server duration without making tests unnecessarily slow (inject a wait function/clock or use a minimal duration seam)
- [x] 1.5 Add tests for context cancellation during FLOOD_WAIT and no sleep after the final attempt
- [x] 1.6 Preserve tests for bounded retry count, non-FLOOD_WAIT exponential backoff, and wrapped final errors

## 2. Apply retry to Telegram history and index operations (TDD)

- [x] 2.1 Wrap each `MessagesGetReplies` page request in `ListFiles` with `retry.WithRetry`, retrying the same `offsetID` after FLOOD_WAIT
- [x] 2.2 Wrap each `MessagesGetReplies` request in `ListIndexMessageIDs` with the same retry policy
- [x] 2.3 Wrap `getLatestTopicMessage` history lookup with the same retry policy
- [x] 2.4 Add mock-invoker tests where a page/latest-message request returns FLOOD_WAIT once and succeeds on retry
- [x] 2.5 Add tests confirming persistent FLOOD_WAIT returns after the configured attempt limit and does not duplicate collected pages

## 3. Remove Takeout pagination (TDD)

- [x] 3.1 Remove Takeout state, deferred `AccountFinishTakeoutSession`, `AccountInitTakeoutSessionRequest`, and `InvokeWithTakeoutRequest` from `ListFiles`
- [x] 3.2 Remove the `totalMessages > 3000` threshold and keep normal `MessagesGetReplies` pagination for every topic size
- [x] 3.3 Delete or rewrite Takeout-only tests to assert normal pagination is used for topics with high reported counts
- [x] 3.4 Run static search to confirm no production Takeout listing code remains

## 4. Error handling and observability (TDD)

- [x] 4.1 Log the operation name and server-provided wait duration when a FLOOD_WAIT retry is scheduled
- [x] 4.2 Preserve context cancellation/deadline behavior while waiting and return context errors promptly
- [x] 4.3 Verify upload/download setup operations automatically gain FLOOD_WAIT handling through the shared helper
- [x] 4.4 Add regression coverage for stale-index discovery invoked by index migration and post-sync rebuild

## 5. Verification and documentation

- [x] 5.1 Run `go test ./...` and `go vet ./...`
- [x] 5.2 Update README with automatic FLOOD_WAIT waiting and the removal of Takeout-specific behavior
- [x] 5.3 Update any retry/history comments that still describe the Takeout threshold
- [x] 5.4 Make conventional commits for retry core, Telegram pagination, Takeout removal, and docs/verification
- [x] 5.5 Run `openspec validate handle-flood-wait-retries` and fix any reported issues
