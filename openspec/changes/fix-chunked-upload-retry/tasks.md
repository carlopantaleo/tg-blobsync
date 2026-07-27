## 1. Setup

- [x] 1.1 Create and checkout feature branch `feature/fix-chunked-upload-retry`
- [x] 1.2 Commit milestone: Setup branch

## 2. Testing (Red Phase)

- [ ] 2.1 Write failing unit tests in `internal/adapter/telegram/files_test.go` simulating a chunked upload failure and verifying the retry behavior.
- [ ] 2.2 Run the tests and ensure they fail (Red).
- [ ] 2.3 Commit milestone: Add failing tests for chunked upload retry

## 3. Implementation (Green Phase)

- [ ] 3.1 Locate the chunked upload loop in `internal/adapter/telegram/files.go` (inside `UploadFile`).
- [ ] 3.2 Wrap the chunk upload API call (`b.client.UploadBigFile`) with `adapter.WithRetry`, similar to the single-file upload logic.
- [ ] 3.3 Ensure error handling properly bubbles up the error if all retries fail.
- [ ] 3.4 Run the tests and ensure they pass (Green).
- [ ] 3.5 Commit milestone: Implement chunked upload retry mechanism

## 4. Refactoring (Refactor Phase)

- [ ] 4.1 Refactor code if necessary to ensure it's clean and readable (SOLID, DRY).
- [ ] 4.2 Run tests again to ensure they still pass.
- [ ] 4.3 Commit milestone: Refactor chunked upload retry code
