package store

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"
)

// LoginChecksPerHour is users-and-access R53's limit per IP address on Login's email step.
const LoginChecksPerHour = 100

type LoginCheckParams struct {
	IPHash []byte
	At     time.Time
}

// Check applies users-and-access R53 to the rolling-hour usage. A refused check answers
// when the oldest counted check leaves the hour; refused checks are never counted, so a
// scan cannot extend its own block.
func (usage LoginCheckUsageRow) Check() (allowed bool, retryAt time.Time) {
	if usage.CheckCount < LoginChecksPerHour {
		return true, time.Time{}
	}
	return false, usage.OldestAt.Add(time.Hour)
}

// ReserveLoginCheck counts one email check for the IP, unless the limit refuses it.
func (s *Store) ReserveLoginCheck(ctx context.Context, arg LoginCheckParams) (allowed bool, retryAt time.Time, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		// Same advisory-lock query as the R43 ledger, in its own key namespace.
		if err := q.LockEmailRequestKey(ctx, "login-check:"+hex.EncodeToString(arg.IPHash)); err != nil {
			return fmt.Errorf("lock login check allowance: %w", err)
		}
		hourStart := arg.At.Add(-time.Hour)
		if err := q.PruneLoginChecks(ctx, hourStart); err != nil {
			return fmt.Errorf("prune login checks: %w", err)
		}
		usage, err := q.LoginCheckUsage(ctx, LoginCheckUsageParams{At: arg.At, IpHash: arg.IPHash, HourStart: hourStart})
		if err != nil {
			return fmt.Errorf("read login check usage: %w", err)
		}
		if allowed, retryAt = usage.Check(); !allowed {
			return nil
		}
		if err := q.RecordLoginCheck(ctx, RecordLoginCheckParams{IpHash: arg.IPHash, CheckedAt: arg.At}); err != nil {
			return fmt.Errorf("record login check: %w", err)
		}
		return nil
	})
	if err != nil {
		return false, time.Time{}, err
	}
	return allowed, retryAt, nil
}
