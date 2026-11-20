package store

import (
	"context"
	"fmt"
	"time"
)

// The limits on failed logins (users-and-access R55, R56; business ADR 0017).
const (
	EmailFailureLimit  = 5
	EmailFailureWindow = 15 * time.Minute
	IPFailureLimit     = 50
	IPFailureWindow    = time.Hour
)

// LoginBlockParams names who is logging in: the email as typed, the IP address and the app,
// which counts separately (users-and-access R57).
type LoginBlockParams struct {
	App, Email, IP string
	At             time.Time
}

// LoginBlockedUntil says until when logins for the email or from the address are blocked,
// or the zero time when they are not (users-and-access R55-R57). Blocked attempts are
// never failures, so they cannot extend a block.
func (s *Store) LoginBlockedUntil(ctx context.Context, arg LoginBlockParams) (time.Time, error) {
	// A block ends 15 minutes after the last failure, and 5 failures within 15 minutes of
	// each other start it, so no failure older than 30 minutes matters.
	byEmail, err := s.RecentFailedLoginsByEmail(ctx, RecentFailedLoginsByEmailParams{App: arg.App, Email: arg.Email, Since: arg.At.Add(-2 * EmailFailureWindow)})
	if err != nil {
		return time.Time{}, fmt.Errorf("recent failed logins by email: %w", err)
	}
	byIP, err := s.RecentFailedLoginsByIP(ctx, RecentFailedLoginsByIPParams{App: arg.App, Ip: arg.IP, Since: arg.At.Add(-IPFailureWindow)})
	if err != nil {
		return time.Time{}, fmt.Errorf("recent failed logins by ip: %w", err)
	}
	return BlockedUntil(byEmail, byIP, arg.At), nil
}

// BlockedUntil applies R55 and R56 to the newest failures, newest first: at most 5 for the
// email and 50 for the address. It is the one place the rules live, for the database and
// the test fake alike.
func BlockedUntil(byEmail, byIP []time.Time, now time.Time) time.Time {
	var until time.Time
	// R55: the 5th-newest failure within 15 minutes of the newest starts a block that
	// ends 15 minutes after the newest.
	if len(byEmail) >= EmailFailureLimit && byEmail[0].Sub(byEmail[EmailFailureLimit-1]) < EmailFailureWindow {
		until = byEmail[0].Add(EmailFailureWindow)
	}
	// R56: while the 50th-newest is still within the hour, 50 remain; the block ends when
	// it leaves the hour.
	if len(byIP) >= IPFailureLimit {
		if end := byIP[IPFailureLimit-1].Add(IPFailureWindow); end.After(until) {
			until = end
		}
	}
	if !until.After(now) {
		return time.Time{}
	}
	return until
}
