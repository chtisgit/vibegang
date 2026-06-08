package agent

import (
	"context"
	"log"
	"time"
)

// Backoff handles exponential backoff for generation errors in the agent loop.
type Backoff struct {
	attempts  int
	intervals []time.Duration
}

// NewBackoff creates a new Backoff instance with the specified policy.
func NewBackoff(intervals []time.Duration) *Backoff {
	return &Backoff{
		attempts:  0,
		intervals: intervals,
	}
}

// Increment increases the backoff level.
func (b *Backoff) Increment() {
	if b.attempts < len(b.intervals) {
		b.attempts++
	}
}

// Reset resets the backoff level to zero.
func (b *Backoff) Reset() {
	b.attempts = 0
}

// Wait blocks for the duration corresponding to the current backoff level,
// or until the context is canceled.
func (b *Backoff) Wait(ctx context.Context) error {
	if b.attempts == 0 {
		return nil
	}
	duration := b.intervals[b.attempts-1]
	log.Printf("Agent waits for %v due to backoff policy", duration)
	select {
	case <-time.After(duration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
