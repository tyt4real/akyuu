package store

import (
	"context"
	"fmt"
	"time"
)

// RecordSiteSuccess resets failure counters and stamps the last successful poll.
func (s *Store) RecordSiteSuccess(ctx context.Context, siteID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sites SET last_successful_poll = now(), consecutive_failures = 0,
		                 last_error = NULL, circuit_open = FALSE, circuit_until = NULL
		WHERE id=$1`, siteID)
	if err != nil {
		return fmt.Errorf("store: record site success: %w", err)
	}
	return nil
}

// RecordSiteFailure increments the consecutive-failure counter and stores the
// error for observability. Returns the new failure count so the scheduler can
// trip the circuit breaker.
func (s *Store) RecordSiteFailure(ctx context.Context, siteID int64, errMsg string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		UPDATE sites SET consecutive_failures = consecutive_failures + 1, last_error = $2
		WHERE id=$1 RETURNING consecutive_failures`, siteID, errMsg).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: record site failure: %w", err)
	}
	return n, nil
}

// SetCircuitOpen opens the circuit for a site until the given time.
func (s *Store) SetCircuitOpen(ctx context.Context, siteID int64, until time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sites SET circuit_open = TRUE, circuit_until = $2 WHERE id=$1`, siteID, until)
	if err != nil {
		return fmt.Errorf("store: open circuit for site %d: %w", siteID, err)
	}
	return nil
}

// CircuitStatus reports whether a site's circuit is currently open.
func (s *Store) CircuitStatus(ctx context.Context, siteID int64) (open bool, until *time.Time, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT circuit_open, circuit_until FROM sites WHERE id=$1`, siteID).Scan(&open, &until)
	if err != nil {
		return false, nil, fmt.Errorf("store: circuit status: %w", err)
	}
	if open && until != nil && until.Before(time.Now()) {
		return false, until, nil
	}
	return open, until, nil
}
