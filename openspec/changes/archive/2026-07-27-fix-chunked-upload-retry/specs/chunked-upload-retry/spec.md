## ADDED Requirements

### Requirement: Retry chunked uploads on failure
The system SHALL retry chunked uploads in the Telegram adapter when encountering temporary errors, using the existing retry mechanisms (`WithRetry`).

#### Scenario: Successful upload after failure
- **WHEN** a chunk upload fails due to a temporary network issue or API limit
- **THEN** the system retries the chunk upload according to the configured retry policy before giving up.

#### Scenario: Permanent failure after retries
- **WHEN** a chunk upload consistently fails beyond the configured retry attempts
- **THEN** the system fails the entire upload process and returns an error.
