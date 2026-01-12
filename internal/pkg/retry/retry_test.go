package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWithRetry(t *testing.T) {
	tests := []struct {
		name        string
		maxRetries  int
		baseDelay   time.Duration
		op          func() (int, error) // Returns attempts made and error
		expectedErr bool
		expectedAtt int
	}{
		{
			name:       "Success on first attempt",
			maxRetries: 3,
			baseDelay:  1 * time.Millisecond,
			op: func() func() (int, error) {
				attempts := 0
				return func() (int, error) {
					attempts++
					return attempts, nil
				}
			}(),
			expectedErr: false,
			expectedAtt: 1,
		},
		{
			name:       "Success after retries",
			maxRetries: 3,
			baseDelay:  1 * time.Millisecond,
			op: func() func() (int, error) {
				attempts := 0
				return func() (int, error) {
					attempts++
					if attempts < 3 {
						return attempts, errors.New("fail")
					}
					return attempts, nil
				}
			}(),
			expectedErr: false,
			expectedAtt: 3,
		},
		{
			name:       "Fail after max retries",
			maxRetries: 3,
			baseDelay:  1 * time.Millisecond,
			op: func() func() (int, error) {
				attempts := 0
				return func() (int, error) {
					attempts++
					return attempts, errors.New("persistent fail")
				}
			}(),
			expectedErr: true,
			expectedAtt: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Wrapper to capture attempts count from the closure
			var attempts int
			op := func() error {
				att, err := tt.op()
				attempts = att
				return err
			}

			err := WithRetry(ctx, "test", op, tt.maxRetries, tt.baseDelay)

			if (err != nil) != tt.expectedErr {
				t.Errorf("WithRetry() error = %v, expectedErr %v", err, tt.expectedErr)
			}

			if attempts != tt.expectedAtt {
				t.Errorf("WithRetry() attempts = %v, expected %v", attempts, tt.expectedAtt)
			}
		})
	}
}

func TestWithRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	maxRetries := 5
	baseDelay := 10 * time.Millisecond

	// Cancel context immediately
	cancel()

	op := func() error {
		return errors.New("should not be called or should fail immediately")
	}

	err := WithRetry(ctx, "test-cancel", op, maxRetries, baseDelay)
	if err == nil {
		t.Error("WithRetry() expected error due to context cancellation, got nil")
	}
}
