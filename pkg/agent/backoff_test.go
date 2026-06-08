package agent

import (
	"context"
	"testing"
	"time"
)

func TestBackoff_IncrementAndReset(t *testing.T) {
	intervals := []time.Duration{
		30 * time.Second,
		2 * time.Minute,
		10 * time.Minute,
		20 * time.Minute,
	}
	b := NewBackoff(intervals)

	// Initial state: 0 attempts, Wait should return immediately (0 duration)
	start := time.Now()
	err := b.Wait(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Errorf("expected no wait for attempt 0, but took %v", time.Since(start))
	}

	// 1st increment -> 30s interval.
	b.Increment()
	if b.attempts != 1 {
		t.Errorf("expected attempts to be 1, got %d", b.attempts)
	}

	// Verify intervals are correct.
	expectedIntervals := []time.Duration{
		30 * time.Second,
		2 * time.Minute,
		10 * time.Minute,
		20 * time.Minute,
	}
	for i, expected := range expectedIntervals {
		if b.intervals[i] != expected {
			t.Errorf("expected interval %d to be %v, got %v", i, expected, b.intervals[i])
		}
	}

	// Increment multiple times and ensure it caps at intervals length.
	b.Increment() // 2
	b.Increment() // 3
	b.Increment() // 4
	b.Increment() // 5 (capped)
	if b.attempts != 4 {
		t.Errorf("expected attempts to cap at 4, got %d", b.attempts)
	}

	// Reset
	b.Reset()
	if b.attempts != 0 {
		t.Errorf("expected attempts to reset to 0, got %d", b.attempts)
	}
}

func TestBackoff_WaitCancellation(t *testing.T) {
	b := NewBackoff([]time.Duration{30 * time.Second})
	b.Increment() // 30s wait

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := b.Wait(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Wait did not abort on context cancel quickly enough, took %v", elapsed)
	}
}
