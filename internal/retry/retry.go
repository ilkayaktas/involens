package retry

import (
	"context"
	"math/rand"
	"strings"
	"time"
)

// transientMarkers are substrings that indicate a transient (retryable) error.
var transientMarkers = []string{"429", "500", "501", "502", "503", "504", "rate limit"}

// IsTransient reports whether err is a transient error that is safe to retry.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range transientMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// Do calls fn up to maxAttempts times (1 initial + maxAttempts-1 retries).
// It retries only when IsTransient returns true for the returned error.
// Backoff: base * 2^attempt + random jitter up to jitterMax.
func Do(ctx context.Context, maxAttempts int, base, jitterMax time.Duration, fn func() error) error {
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !IsTransient(err) {
			return err
		}
		if attempt == maxAttempts-1 {
			break
		}
		backoff := base * (1 << uint(attempt))
		jitter := time.Duration(rand.Int63n(int64(jitterMax)))
		wait := backoff + jitter

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return err
}
