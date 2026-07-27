package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestParseFloodWait(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantSeconds int
		wantMatch   bool
	}{
		{
			name:        "canonical",
			err:         errors.New("rpc error code 420: FLOOD_WAIT (25)"),
			wantSeconds: 25,
			wantMatch:   true,
		},
		{
			name:        "wrapped",
			err:         errors.New("something went wrong: FLOOD_WAIT (7)"),
			wantSeconds: 7,
			wantMatch:   true,
		},
		{
			name:        "malformed",
			err:         errors.New("FLOOD_WAIT (abc)"),
			wantSeconds: 0,
			wantMatch:   false,
		},
		{
			name:        "unrelated",
			err:         errors.New("some other error"),
			wantSeconds: 0,
			wantMatch:   false,
		},
		{
			name:        "nil",
			err:         nil,
			wantSeconds: 0,
			wantMatch:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSeconds, gotMatch := ParseFloodWait(tt.err)
			if gotSeconds != tt.wantSeconds || gotMatch != tt.wantMatch {
				t.Errorf("ParseFloodWait() = (%v, %v), want (%v, %v)", gotSeconds, gotMatch, tt.wantSeconds, tt.wantMatch)
			}
		})
	}
}

// Mock clock for testing
type mockClock struct {
	slept []time.Duration
}

func (m *mockClock) Sleep(ctx context.Context, d time.Duration) error {
	m.slept = append(m.slept, d)
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func TestWithRetry_FloodWait(t *testing.T) {
	clock := &mockClock{}
	oldSleep := sleepFunc
	sleepFunc = clock.Sleep
	defer func() { sleepFunc = oldSleep }()

	attempts := 0
	op := func() error {
		attempts++
		if attempts == 1 {
			return errors.New("FLOOD_WAIT (5)")
		}
		return nil
	}

	err := WithRetry(context.Background(), "test_flood", op, 3, time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got: %d", attempts)
	}
	if len(clock.slept) != 1 || clock.slept[0] != 5*time.Second {
		t.Fatalf("expected to sleep for 5s, got: %v", clock.slept)
	}
}

func TestWithRetry_FloodWait_Cancel(t *testing.T) {
	clock := &mockClock{}
	oldSleep := sleepFunc
	sleepFunc = func(ctx context.Context, d time.Duration) error {
		clock.slept = append(clock.slept, d)
		return context.Canceled
	}
	defer func() { sleepFunc = oldSleep }()

	op := func() error {
		return errors.New("FLOOD_WAIT (10)")
	}

	err := WithRetry(context.Background(), "test_flood_cancel", op, 3, time.Millisecond)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if len(clock.slept) != 1 || clock.slept[0] != 10*time.Second {
		t.Fatalf("expected to try to sleep for 10s, got: %v", clock.slept)
	}
}
