## ADDED Requirements

### Requirement: Parse Telegram FLOOD_WAIT duration
The retry subsystem SHALL recognize Telegram rate-limit errors containing `FLOOD_WAIT (N)` and SHALL extract `N` as a positive number of seconds. Errors without that pattern SHALL be treated as non-FLOOD_WAIT errors.

#### Scenario: Canonical FLOOD_WAIT error is parsed
- **WHEN** an operation returns `rpc error code 420: FLOOD_WAIT (25)`
- **THEN** the retry subsystem SHALL identify it as a FLOOD_WAIT
- **AND** it SHALL extract a wait duration of 25 seconds

#### Scenario: Wrapped FLOOD_WAIT error is parsed
- **WHEN** an operation returns an error wrapping or prefixing `FLOOD_WAIT (7)`
- **THEN** the retry subsystem SHALL identify it as a FLOOD_WAIT
- **AND** it SHALL extract a wait duration of 7 seconds

#### Scenario: Other error is not parsed as FLOOD_WAIT
- **WHEN** an operation returns an error without a valid `FLOOD_WAIT (N)` token
- **THEN** the retry subsystem SHALL not classify it as FLOOD_WAIT

### Requirement: Retry waits for Telegram-provided duration
When a retryable operation fails with FLOOD_WAIT, the retry helper SHALL wait for the extracted number of seconds before the next attempt instead of using the normal exponential delay. The wait SHALL be interruptible by the supplied context.

#### Scenario: FLOOD_WAIT operation succeeds after waiting
- **WHEN** an operation fails with `FLOOD_WAIT (N)` and succeeds on the next attempt
- **THEN** the helper SHALL invoke the next attempt only after at least N seconds
- **AND** the helper SHALL return nil

#### Scenario: Context cancellation interrupts FLOOD_WAIT
- **WHEN** an operation fails with `FLOOD_WAIT (N)` and the context is cancelled during the wait
- **THEN** the helper SHALL stop waiting
- **AND** it SHALL return the context error without invoking another attempt

#### Scenario: Final FLOOD_WAIT does not cause an extra sleep
- **WHEN** every allowed attempt fails and the final failure is `FLOOD_WAIT (N)`
- **THEN** the helper SHALL return the operation error after the final attempt
- **AND** it SHALL not sleep for another retry

### Requirement: Existing retry behavior remains bounded
The retry helper SHALL preserve its configured maximum attempt count, SHALL retry non-FLOOD_WAIT transient errors using the existing backoff behavior, and SHALL return the operation name with the final error cause.

#### Scenario: Non-FLOOD_WAIT errors use exponential backoff
- **WHEN** an operation repeatedly fails without a FLOOD_WAIT duration
- **THEN** the helper SHALL use the configured exponential backoff between attempts
- **AND** it SHALL stop after the configured maximum number of attempts

#### Scenario: Context cancellation prevents another attempt
- **WHEN** the parent context is cancelled after an operation failure
- **THEN** the helper SHALL return the context error
- **AND** it SHALL not invoke the operation again
