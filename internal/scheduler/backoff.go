package scheduler

import (
	"math/rand"
	"time"
)

// backoffDelay computes the retry delay for a failed job: exponential with a
// random jitter so concurrent failures do not pile up on the same tick.
func backoffDelay(attempt int, base time.Duration, max time.Duration) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	if attempt <= 1 {
		attempt = 1
	}
	// Cap the exponent so the delay never overflows.
	e := attempt - 1
	if e > 30 {
		e = 30
	}
	delay := base * time.Duration(1<<e)
	if delay > max {
		delay = max
	}
	// +/- 25% jitter.
	j := time.Duration(rand.Int63n(int64(delay/2))) - delay/4
	return delay + j
}

// circuitCooldown computes how long a circuit stays open after consecutive
// failures. Doubles per failure threshold crossed, bounded.
func circuitCooldown(pollInterval time.Duration, failures int) time.Duration {
	base := pollInterval
	if base < time.Minute {
		base = time.Minute
	}
	e := failures - 1
	if e > 5 {
		e = 5
	}
	d := base * time.Duration(1<<e)
	if d > 24*time.Hour {
		d = 24 * time.Hour
	}
	return d
}
