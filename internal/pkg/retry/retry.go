package retry

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"time"
)

// Operation represents a function that can be retried.
type Operation func() error

// WithRetry executes the given operation with exponential backoff.
func WithRetry(ctx context.Context, name string, op Operation, maxRetries int, baseDelay time.Duration) error {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			delay := time.Duration(math.Pow(2, float64(attempt-2))) * baseDelay
			log.Printf("[!] Retry %d/%d for %s after %v...", attempt, maxRetries, name, delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		err := op()
		if err == nil {
			return nil
		}
		lastErr = err
		log.Printf("[!] Error during %s (attempt %d/%d): %v", name, attempt, maxRetries, err)

		// Don't retry if context is cancelled or deadline exceeded
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
	}
	return fmt.Errorf("%s failed after %d attempts: %w", name, maxRetries, lastErr)
}
