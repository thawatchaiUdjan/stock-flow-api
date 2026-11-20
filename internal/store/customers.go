package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ErrCustomerEmailTaken refuses a sign-up for an email a Customer has, in any letter case
// (customers R3, R6). A Deleted Customer's email is free again.
var ErrCustomerEmailTaken = errors.New("the email has a Customer")

// SignupCodeParams is a Verification code for an email that has no Customer yet (customers R5, R8).
type SignupCodeParams struct {
	Email     string
	CodeHash  []byte
	ExpiresAt time.Time
}

// SignupGrantParams names a sign-up grant, which lets its holder complete the sign-up.
type SignupGrantParams struct {
	TokenHash []byte
	At        time.Time
}

// CompleteSignupParams is the last step of a sign-up (customers R2, R4, R5, R11): the grant
// the code was exchanged for, and what the Customer chose.
type CompleteSignupParams struct {
	GrantHash                 []byte
	At                        time.Time
	Name, Phone, PasswordHash string
}

// signupLockKey serializes everything that happens to one email's sign-up, in its own
// namespace of the advisory locks the email ledgers use.
func signupLockKey(email string) string { return "signup:" + strings.ToLower(email) }

// IssueSignupCode stores the Verification code, replacing the email's earlier one but not
// its failed attempts or its block (customers R8). It answers ErrCustomerEmailTaken when the
// email has a Customer, which is checked under the email's lock, so a Customer made meanwhile
// is not missed (customers R3, R6).
func (s *Store) IssueSignupCode(ctx context.Context, arg SignupCodeParams) error {
	return InTx(ctx, s.pool, func(q *Queries) error {
		if err := q.LockEmailRequestKey(ctx, signupLockKey(arg.Email)); err != nil {
			return fmt.Errorf("lock sign-up email: %w", err)
		}
		if _, err := q.GetCustomerByEmail(ctx, arg.Email); err == nil {
			return ErrCustomerEmailTaken
		} else if !errors.Is(err, ErrNotFound) {
			return fmt.Errorf("find Customer: %w", err)
		}
		if err := q.IssueSignupCredential(ctx, IssueSignupCredentialParams{
			Email: arg.Email, Kind: FirstTimeCode, TokenHash: arg.CodeHash, ExpiresAt: arg.ExpiresAt,
		}); err != nil {
			return fmt.Errorf("issue sign-up code: %w", err)
		}
		return nil
	})
}

// VerifySignupCode exchanges the email's code for a sign-up grant, consuming the code
// (customers R5, R8). A wrong code counts as a failed attempt; five block codes for ten
// minutes. It reports whether the code was right. The rules are the Users' (VerifyCode).
func (s *Store) VerifySignupCode(ctx context.Context, arg VerifyFirstTimeCodeParams) (verified bool, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		if err := q.LockEmailRequestKey(ctx, signupLockKey(arg.Email)); err != nil {
			return fmt.Errorf("lock sign-up email: %w", err)
		}
		row, err := q.GetSignupCredential(ctx, arg.Email)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read sign-up code: %w", err)
		}
		next, accepted := EmailCredential{
			Kind: row.Kind, TokenHash: row.TokenHash, ExpiresAt: row.ExpiresAt, FailedAttempts: row.FailedAttempts, BlockedUntil: row.BlockedUntil,
		}.VerifyCode(arg)
		if accepted {
			if err := q.IssueSignupCredential(ctx, IssueSignupCredentialParams{
				Email: arg.Email, Kind: next.Kind, TokenHash: next.TokenHash, ExpiresAt: next.ExpiresAt,
			}); err != nil {
				return fmt.Errorf("exchange code for sign-up grant: %w", err)
			}
		}
		if err := q.SetSignupCodeAttempts(ctx, SetSignupCodeAttemptsParams{
			Email: arg.Email, FailedAttempts: next.FailedAttempts, BlockedUntil: next.BlockedUntil,
		}); err != nil {
			return fmt.Errorf("save sign-up code attempts: %w", err)
		}
		verified = accepted
		return nil
	})
	return verified && err == nil, err
}

// ReadSignupGrant answers ErrNotFound unless the grant is live. It lets the API refuse a bad
// grant before it hashes a password; CompleteSignup checks again under the lock.
func (s *Store) ReadSignupGrant(ctx context.Context, arg SignupGrantParams) error {
	if _, err := s.GetSignupGrant(ctx, GetSignupGrantParams{TokenHash: arg.TokenHash, At: arg.At}); err != nil {
		return fmt.Errorf("read sign-up grant: %w", err)
	}
	return nil
}

// CompleteSignup uses up the grant and creates the Customer, active and with their password,
// and writes the sign-up's entry, with the Customer as the actor, in the same transaction
// (customers R5, activity-log R3, R4). It answers ErrNotFound when the grant does not work
// and ErrCustomerEmailTaken when the email has a Customer by now.
func (s *Store) CompleteSignup(ctx context.Context, arg CompleteSignupParams, by ActivityBy) (created Customer, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		grant, err := q.GetSignupGrant(ctx, GetSignupGrantParams{TokenHash: arg.GrantHash, At: arg.At})
		if err != nil {
			return fmt.Errorf("find sign-up grant: %w", err)
		}
		if err := q.LockEmailRequestKey(ctx, signupLockKey(grant.Email)); err != nil {
			return fmt.Errorf("lock sign-up email: %w", err)
		}
		// The grant may have been used while this waited for the lock.
		if _, err := q.GetSignupGrant(ctx, GetSignupGrantParams{TokenHash: arg.GrantHash, At: arg.At}); err != nil {
			return fmt.Errorf("find sign-up grant: %w", err)
		}
		created, err = q.CreateCustomer(ctx, CreateCustomerParams{
			Name: arg.Name, Email: grant.Email, Phone: arg.Phone, PasswordHash: pgtype.Text{String: arg.PasswordHash, Valid: true}, CreatedAt: arg.At,
		})
		if errors.Is(err, ErrNotFound) {
			return ErrCustomerEmailTaken
		}
		if err != nil {
			return fmt.Errorf("create Customer: %w", err)
		}
		if err := q.DeleteSignupCredential(ctx, grant.Email); err != nil {
			return fmt.Errorf("use up sign-up grant: %w", err)
		}
		return q.addActivity(ctx, CustomerCreated(created, by.AsCustomer(created)))
	})
	return created, err
}

// StartCustomerSessionParams is a Storefront login that passed its password check: the
// session to start and the entry that records it. Entry.Result is set by StartCustomerSession.
type StartCustomerSessionParams struct {
	Session CreateCustomerSessionParams
	Entry   AddLoginLogEntryParams
}

// StartCustomerSession starts the session and writes the entry in one transaction, as
// StartSession does for a User (customers R9, R10, users-and-access R36). When the Customer
// changed while the password was checked, no session starts and the entry is a failed login.
// It reports whether a session started.
func (s *Store) StartCustomerSession(ctx context.Context, arg StartCustomerSessionParams) (started bool, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		n, err := q.CreateCustomerSession(ctx, arg.Session)
		if err != nil {
			return fmt.Errorf("create Customer session: %w", err)
		}
		started = n > 0
		arg.Entry.Result = ResultFailedLogin
		if started {
			arg.Entry.Result = ResultLogin
		}
		if err := q.AddLoginLogEntry(ctx, arg.Entry); err != nil {
			return fmt.Errorf("add login log entry: %w", err)
		}
		return nil
	})
	return started && err == nil, err
}

// EndCustomerSessionParams is a Storefront logout: the session to end and the entry that
// records it. Entry.Result is set by EndCustomerSession.
type EndCustomerSessionParams struct {
	TokenHash []byte
	Entry     AddLoginLogEntryParams
}

// EndCustomerSession ends the session and writes the entry in one transaction (customers
// R10). A session already gone writes no entry: only the call that ended it logged out.
func (s *Store) EndCustomerSession(ctx context.Context, arg EndCustomerSessionParams) error {
	return InTx(ctx, s.pool, func(q *Queries) error {
		ended, err := q.DeleteCustomerSession(ctx, arg.TokenHash)
		if err != nil {
			return fmt.Errorf("delete Customer session: %w", err)
		}
		if ended == 0 {
			return nil
		}
		arg.Entry.Result = ResultLogout
		if err := q.AddLoginLogEntry(ctx, arg.Entry); err != nil {
			return fmt.Errorf("add login log entry: %w", err)
		}
		return nil
	})
}
