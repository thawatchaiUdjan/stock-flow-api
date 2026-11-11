package store

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
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
				return fmt.Errorf("lock email request allowance: %w", err)
			}
		}
		if err := q.PruneEmailRequests(ctx, arg.At.Add(-time.Hour)); err != nil {
			return fmt.Errorf("prune email request history: %w", err)
		}
		usage, err := q.EmailRequestUsage(ctx, EmailRequestUsageParams{
			EmailHash: arg.EmailHash, IpHash: arg.IPHash,
			HourStart: arg.At.Add(-time.Hour), MinuteStart: arg.At.Add(-time.Minute),
		})
		if err != nil {
			return fmt.Errorf("read email request usage: %w", err)
		}
		allowed = usage.AllowsRequest()
		if !allowed {
			return nil
		}
		if err := q.RecordEmailRequest(ctx, RecordEmailRequestParams{EmailHash: arg.EmailHash, IpHash: arg.IPHash, RequestedAt: arg.At}); err != nil {
			return fmt.Errorf("reserve email request: %w", err)
		}
		return nil
	})
	return allowed && err == nil, err
}

func lockPasswordUser(ctx context.Context, q *Queries, id int64) (User, error) {
	if err := q.LockSetupLink(ctx, id); err != nil {
		return User{}, fmt.Errorf("lock legacy setup link: %w", err)
	}
	user, err := q.LockEmailAuthUser(ctx, id)
	if err != nil {
		return User{}, fmt.Errorf("lock email access User: %w", err)
	}
	return user, nil
}

// IssuePasswordCredential rechecks eligibility under the User lock, so a concurrent
// deactivation or password change cannot leave usable instructions (users-and-access R32, R48).
func (s *Store) IssuePasswordCredential(ctx context.Context, arg EmailCredentialParams) (issued User, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		user, err := q.GetUserByEmail(ctx, arg.Email)
		if err != nil {
			return fmt.Errorf("find email credential User: %w", err)
		}
		user, err = lockPasswordUser(ctx, q, user.ID)
		if err != nil {
			return err
		}
		if !user.CanUseEmailCredential(arg.Kind) {
			return ErrNotFound
		}
		if err := q.IssueEmailCredential(ctx, IssueEmailCredentialParams{
			UserID: user.ID, Kind: arg.Kind, TokenHash: arg.TokenHash, ExpiresAt: arg.ExpiresAt,
		}); err != nil {
			return fmt.Errorf("issue email credential: %w", err)
		}
		issued = user
		return nil
	})
	return issued, err
}

// VerifyFirstTimeCode consumes the code when it exchanges it for a grant (users-and-access R42).
func (s *Store) VerifyFirstTimeCode(ctx context.Context, arg VerifyFirstTimeCodeParams) (verified bool, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		user, err := q.GetUserByEmail(ctx, arg.Email)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find code verification User: %w", err)
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
			return fmt.Errorf("read code verification credential: %w", err)
		}
		if !user.CanUseEmailCredential(FirstTimeCode) {
			return nil
		}
		next, accepted := credential.VerifyCode(arg)
		if !accepted {
			if err := q.SetEmailCodeAttempts(ctx, SetEmailCodeAttemptsParams{
				UserID: user.ID, FailedAttempts: next.FailedAttempts, BlockedUntil: next.BlockedUntil,
			}); err != nil {
				return fmt.Errorf("save code verification failures: %w", err)
			}
			return nil
		}
		if err := q.IssueEmailCredential(ctx, IssueEmailCredentialParams{
			UserID: user.ID, Kind: next.Kind, TokenHash: next.TokenHash, ExpiresAt: next.ExpiresAt,
		}); err != nil {
			return fmt.Errorf("exchange code for password grant: %w", err)
		}
		if err := q.SetEmailCodeAttempts(ctx, SetEmailCodeAttemptsParams{
			UserID: user.ID, FailedAttempts: next.FailedAttempts, BlockedUntil: next.BlockedUntil,
		}); err != nil {
			return fmt.Errorf("save password grant attempt state: %w", err)
		}
		verified = true
		return nil
	})
	return verified && err == nil, err
}

// ReadEmailPasswordUser validates before the expensive password hash. Completion
// rechecks under lock; this read is not authority to change a password.
func (s *Store) ReadEmailPasswordUser(ctx context.Context, arg EmailPasswordParams) (User, error) {
	user, err := s.GetEmailCredentialUser(ctx, GetEmailCredentialUserParams{Kind: arg.Kind, TokenHash: arg.TokenHash, At: arg.At})
	if err != nil {
		return User{}, fmt.Errorf("read email password credential: %w", err)
	}
	return user, nil
}

// SetEmailPassword consumes a grant/link, changes the password and removes every
// credential/session atomically (users-and-access R30, R31, R48).
func (s *Store) SetEmailPassword(ctx context.Context, arg EmailPasswordParams) (updated User, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		user, err := q.GetEmailCredentialUser(ctx, GetEmailCredentialUserParams{Kind: arg.Kind, TokenHash: arg.TokenHash, At: arg.At})
		if err != nil {
			return fmt.Errorf("find password completion User: %w", err)
		}
		user, err = lockPasswordUser(ctx, q, user.ID)
		if err != nil {
			return err
		}
		credential, err := q.GetEmailCredential(ctx, user.ID)
		if err != nil {
			return fmt.Errorf("read password completion credential: %w", err)
		}
		if !credential.AllowsPassword(user, arg) {
			return ErrNotFound
		}
		if _, err := q.SetUserPassword(ctx, SetUserPasswordParams{ID: user.ID, PasswordHash: arg.PasswordHash}); err != nil {
			return fmt.Errorf("replace User password: %w", err)
		}
		if err := q.DeleteUserSessions(ctx, user.ID); err != nil {
			return fmt.Errorf("revoke password completion sessions: %w", err)
		}
		if err := q.DeleteSetupLink(ctx, user.ID); err != nil {
			return fmt.Errorf("delete legacy password credential: %w", err)
		}
		if err := q.DeleteEmailCredential(ctx, user.ID); err != nil {
			return fmt.Errorf("consume email password credential: %w", err)
		}
		user.PasswordHash.String, user.PasswordHash.Valid = arg.PasswordHash, true
		updated = user
		return nil
	})
	return updated, err
}
