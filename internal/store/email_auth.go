package store

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"time"
)

const (
	FirstTimeCode = "first-time-code"
	SetupGrant    = "setup-grant"
	ResetLink     = "reset-link"
)

// EmailCredentialParams binds a credential to its User, purpose and expiry.
type EmailCredentialParams struct {
	Email     string
	Kind      string
	TokenHash []byte
	ExpiresAt time.Time
}

// EmailPasswordParams identifies a password-setup grant or recovery link.
type EmailPasswordParams struct {
	Kind         string
	TokenHash    []byte
	At           time.Time
	PasswordHash string
}

type VerifyFirstTimeCodeParams struct {
	Email          string
	CodeHash       []byte
	GrantHash      []byte
	GrantExpiresAt time.Time
	At             time.Time
}

type EmailRequestParams struct {
	EmailHash []byte
	IPHash    []byte
	At        time.Time
}

// ReserveEmailRequest applies shared rolling limits before eligibility checks.
// Unknown addresses consume the same allowance without being persisted in plaintext.
func (s *Store) ReserveEmailRequest(ctx context.Context, arg EmailRequestParams) (allowed bool, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		for _, key := range []string{"email:" + hex.EncodeToString(arg.EmailHash), "ip:" + hex.EncodeToString(arg.IPHash)} {
			if err := q.LockEmailRequestKey(ctx, key); err != nil {
				return err
			}
		}
		if err := q.PruneEmailRequests(ctx, arg.At.Add(-time.Hour)); err != nil {
			return err
		}
		allowance, err := q.EmailRequestAllowed(ctx, EmailRequestAllowedParams{
			EmailHash: arg.EmailHash, IpHash: arg.IPHash,
			HourStart: arg.At.Add(-time.Hour), MinuteStart: arg.At.Add(-time.Minute),
		})
		allowed = allowance.Valid && allowance.Bool
		if err != nil || !allowed {
			return err
		}
		return q.RecordEmailRequest(ctx, RecordEmailRequestParams{EmailHash: arg.EmailHash, IpHash: arg.IPHash, RequestedAt: arg.At})
	})
	return allowed && err == nil, err
}

func lockPasswordUser(ctx context.Context, q *Queries, id int64) (User, error) {
	if err := q.LockSetupLink(ctx, id); err != nil {
		return User{}, err
	}
	return q.LockEmailAuthUser(ctx, id)
}

func emailCredentialEligible(user User, kind string) bool {
	if user.Status != "active" {
		return false
	}
	if kind == ResetLink {
		return user.PasswordHash.Valid
	}
	return !user.PasswordHash.Valid && user.Role != roleOwner
}

// IssuePasswordCredential rechecks eligibility under the User lock, so a concurrent
// deactivation or password change cannot leave usable instructions (R32, R48).
func (s *Store) IssuePasswordCredential(ctx context.Context, arg EmailCredentialParams) (issued User, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		user, err := q.GetUserByEmail(ctx, arg.Email)
		if err != nil {
			return err
		}
		user, err = lockPasswordUser(ctx, q, user.ID)
		if err != nil {
			return err
		}
		if !emailCredentialEligible(user, arg.Kind) {
			return ErrNotFound
		}
		if err := q.IssueEmailCredential(ctx, IssueEmailCredentialParams{
			UserID: user.ID, Kind: arg.Kind, TokenHash: arg.TokenHash, ExpiresAt: arg.ExpiresAt,
		}); err != nil {
			return err
		}
		issued = user
		return nil
	})
	return issued, err
}

// VerifyFirstTimeCode consumes the code when it exchanges it for a grant (R42).
func (s *Store) VerifyFirstTimeCode(ctx context.Context, arg VerifyFirstTimeCodeParams) (verified bool, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		user, err := q.GetUserByEmail(ctx, arg.Email)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		user, err = lockPasswordUser(ctx, q, user.ID)
		if err != nil {
			return err
		}
		credential, err := q.GetEmailCredential(ctx, user.ID)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if !emailCredentialEligible(user, FirstTimeCode) || credential.Kind != FirstTimeCode ||
			!credential.ExpiresAt.After(arg.At) || credential.BlockedUntil.After(arg.At) {
			return nil
		}
		// R41: a completed block restores the allowance; a resend never does.
		if credential.FailedAttempts == 5 {
			credential.FailedAttempts = 0
		}
		if subtle.ConstantTimeCompare(credential.TokenHash, arg.CodeHash) != 1 {
			credential.FailedAttempts++
			if credential.FailedAttempts == 5 {
				credential.BlockedUntil = arg.At.Add(10 * time.Minute)
			}
			return q.SetEmailCodeAttempts(ctx, SetEmailCodeAttemptsParams{
				UserID: user.ID, FailedAttempts: credential.FailedAttempts, BlockedUntil: credential.BlockedUntil,
			})
		}
		if err := q.IssueEmailCredential(ctx, IssueEmailCredentialParams{
			UserID: user.ID, Kind: SetupGrant, TokenHash: arg.GrantHash, ExpiresAt: arg.GrantExpiresAt,
		}); err != nil {
			return err
		}
		verified = true
		return nil
	})
	return verified && err == nil, err
}

// ReadEmailPasswordUser validates before the expensive password hash. Completion
// rechecks under lock; this read is not authority to change a password.
func (s *Store) ReadEmailPasswordUser(ctx context.Context, arg EmailPasswordParams) (User, error) {
	return s.GetEmailCredentialUser(ctx, GetEmailCredentialUserParams{Kind: arg.Kind, TokenHash: arg.TokenHash, At: arg.At})
}

// SetEmailPassword consumes a grant/link, changes the password and removes every
// credential/session atomically (users-and-access R30, R31, R48).
func (s *Store) SetEmailPassword(ctx context.Context, arg EmailPasswordParams) (updated User, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		user, err := q.GetEmailCredentialUser(ctx, GetEmailCredentialUserParams{Kind: arg.Kind, TokenHash: arg.TokenHash, At: arg.At})
		if err != nil {
			return err
		}
		user, err = lockPasswordUser(ctx, q, user.ID)
		if err != nil {
			return err
		}
		credential, err := q.GetEmailCredential(ctx, user.ID)
		if err != nil {
			return err
		}
		if !emailCredentialEligible(user, arg.Kind) || credential.Kind != arg.Kind ||
			!credential.ExpiresAt.After(arg.At) || subtle.ConstantTimeCompare(credential.TokenHash, arg.TokenHash) != 1 {
			return ErrNotFound
		}
		if _, err := q.SetUserPassword(ctx, SetUserPasswordParams{ID: user.ID, PasswordHash: arg.PasswordHash}); err != nil {
			return err
		}
		if err := q.DeleteUserSessions(ctx, user.ID); err != nil {
			return err
		}
		if err := q.DeleteSetupLink(ctx, user.ID); err != nil {
			return err
		}
		if err := q.DeleteEmailCredential(ctx, user.ID); err != nil {
			return err
		}
		user.PasswordHash.String, user.PasswordHash.Valid = arg.PasswordHash, true
		updated = user
		return nil
	})
	return updated, err
}
