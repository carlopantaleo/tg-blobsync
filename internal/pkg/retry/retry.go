package retry

import (
	"context"
	"fmt"
	"log"
	"math"
	"regexp"
	"strconv"
	"time"
)

var floodWaitRegex = regexp.MustCompile(`FLOOD_WAIT \((\d+)\)`)

// ParseFloodWait extracts the duration in seconds from a FLOOD_WAIT error.
func ParseFloodWait(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	matches := floodWaitRegex.FindStringSubmatch(err.Error())
	if len(matches) == 2 {
		seconds, err := strconv.Atoi(matches[1])
		if err == nil {
			return seconds, true
		}
	}
	return 0, false
}

var sleepFunc = func(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Operation represents a function that can be retried.
type Operation func() error

// WithRetry executes the given operation with exponential backoff, handling FLOOD_WAIT explicitly.
func WithRetry(ctx context.Context, name string, op Operation, maxRetries int, baseDelay time.Duration) error {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			var delay time.Duration
			if seconds, isFlood := ParseFloodWait(lastErr); isFlood {
				delay = time.Duration(seconds) * time.Second
				log.Printf("[!] FLOOD_WAIT retry %d/%d for %s after %v...", attempt, maxRetries, name, delay)
			} else {
				delay = time.Duration(math.Pow(2, float64(attempt-2))) * baseDelay
				log.Printf("[!] Retry %d/%d for %s after %v...", attempt, maxRetries, name, delay)
			}

			if err := sleepFunc(ctx, delay); err != nil {
				return err
			}
		}

		err := op()
		if err == nil {
			return nil
		}
		lastErr = err
		log.Printf("[!] Error during %s (attempt %d/%d): %v", name, attempt, maxRetries, err)

		// Don't retry if the parent context is cancelled or deadline exceeded.
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return fmt.Errorf("%s failed after %d attempts: %w", name, maxRetries, lastErr)
}
