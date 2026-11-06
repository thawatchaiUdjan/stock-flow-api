package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrTargetIsOwner refuses any change to the Owner's User, which is managed by a
	// developer and not through the Users pages (users-and-access R18, R34, context ADR 0004).
	ErrTargetIsOwner = errors.New("the target is the Owner")
	// ErrUserInactive refuses a Setup link for an Inactive User (users-and-access R32).
	ErrUserInactive = errors.New("the User is Inactive")
)

const (
	roleOwner      = "owner"
	statusInactive = "inactive"
)

// The results a Login log entry records (users-and-access R36).
const (
	ResultLogin       = "login"
	ResultFailedLogin = "failed-login"
	ResultLogout      = "logout"
)

// StartSessionParams is a login that passed its password check: the session to start and
// the entry that records it. Entry.Result is set by StartSession.
type StartSessionParams struct {
	Session CreateSessionParams
	Entry   AddLoginLogEntryParams
}

// StartSession starts the session and writes the entry in one transaction, so a login is
// never in one without the other (users-and-access R36). When the User changed while the
// password was checked (R16, R31, R34) no session starts and the entry is a failed login
// instead. It reports whether a session started.
func (s *Store) StartSession(ctx context.Context, arg StartSessionParams) (started bool, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		n, err := q.CreateSession(ctx, arg.Session)
		if err != nil {
			return fmt.Errorf("create session: %w", err)
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

// EndSessionParams is a logout: the session to end and the entry that records it.
// Entry.Result is set by EndSession.
type EndSessionParams struct {
	TokenHash []byte
	Entry     AddLoginLogEntryParams
}

// EndSession ends the session and writes the entry in one transaction (users-and-access
// R7, R36). A session already gone, as when two logouts carry one cookie, writes no entry:
// only the call that ended it logged out.
func (s *Store) EndSession(ctx context.Context, arg EndSessionParams) error {
	return InTx(ctx, s.pool, func(q *Queries) error {
		ended, err := q.DeleteSession(ctx, arg.TokenHash)
		if err != nil {
			return fmt.Errorf("delete session: %w", err)
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

// Store is every query, and the rules that take more than one statement. The API runs on
// it rather than on Queries, whose ChangeUser alone would skip those rules.
type Store struct {
	*Queries
	pool *pgxpool.Pool
}

// NewStore returns the Store of pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Queries: New(pool), pool: pool}
}

// lockTarget holds the User until the transaction ends, so two changes to one User run one
// after the other and each statement after this sees what the other committed. A single
// statement can't do that: its snapshot predates the wait, so it would miss a session or a
// Setup link committed meanwhile. The link comes first because SetPassword takes the link
// and then the User: in the other order the two deadlock.
func lockTarget(ctx context.Context, q *Queries, id int64) (LockUserRow, error) {
	if err := q.LockSetupLink(ctx, id); err != nil {
		return LockUserRow{}, fmt.Errorf("lock setup link: %w", err)
	}
	target, err := q.LockUser(ctx, id)
	if err != nil {
		return LockUserRow{}, fmt.Errorf("lock user: %w", err)
	}
	if target.Role == roleOwner {
		return LockUserRow{}, ErrTargetIsOwner
	}
	return target, nil
}

// UpdateUser changes what arg names, in one transaction: an Inactive User has no session
// and no Setup link left (users-and-access R16, R32). It answers ErrNotFound for no such
// User and ErrTargetIsOwner for the Owner.
func (s *Store) UpdateUser(ctx context.Context, arg ChangeUserParams) (updated ChangeUserRow, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		if _, err := lockTarget(ctx, q, arg.ID); err != nil {
			return err
		}
		updated, err = q.ChangeUser(ctx, arg)
		if err != nil {
			return fmt.Errorf("change user: %w", err)
		}
		if updated.Status != statusInactive {
			return nil
		}
		if err := q.DeleteUserSessions(ctx, arg.ID); err != nil {
			return fmt.Errorf("delete sessions: %w", err)
		}
		if err := q.DeleteSetupLink(ctx, arg.ID); err != nil {
			return fmt.Errorf("delete setup link: %w", err)
		}
		return nil
	})
	return updated, err
}

// ResetPassword ends the User's password and sessions and issues arg in place of any
// earlier Setup link, in one transaction (users-and-access R29, R31). It answers
// ErrNotFound for no such User, ErrTargetIsOwner for the Owner (R34) and ErrUserInactive
// for an Inactive User (R32), and then changes nothing.
func (s *Store) ResetPassword(ctx context.Context, arg IssueSetupLinkParams) error {
	return InTx(ctx, s.pool, func(q *Queries) error {
		target, err := lockTarget(ctx, q, arg.UserID)
		if err != nil {
			return err
		}
		if target.Status == statusInactive {
			return ErrUserInactive
		}
		if err := q.ClearPassword(ctx, arg.UserID); err != nil {
			return fmt.Errorf("clear password: %w", err)
		}
		if err := q.DeleteUserSessions(ctx, arg.UserID); err != nil {
			return fmt.Errorf("delete sessions: %w", err)
		}
		if err := q.IssueSetupLink(ctx, arg); err != nil {
			return fmt.Errorf("issue setup link: %w", err)
		}
		return nil
	})
}

// SetPasswordParams is a password being set through the Setup link of TokenHash, at At.
type SetPasswordParams struct {
	PasswordHash string
	TokenHash    []byte
	At           time.Time
}

// SetPassword uses up the Setup link and sets its User's password, in one transaction.
// Setting the Owner's password also ends all the Owner's sessions (users-and-access R34,
// context ADR 0003); no one else's password-set ends any. It returns how many passwords
// were set: 1, or 0 when the link doesn't work.
//
// No session of the Owner survives, however a login overlaps this. The password goes first:
// it waits for every login holding the User (CreateSession's FOR SHARE), and a login that
// comes later waits for this transaction and then finds the password changed. The sessions
// go in a statement of their own, after that wait: its snapshot holds what those logins
// committed, which one statement's snapshot, older than the wait, would miss.
func (s *Store) SetPassword(ctx context.Context, arg SetPasswordParams) (set int64, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		userID, err := q.UseSetupLink(ctx, UseSetupLinkParams{TokenHash: arg.TokenHash, At: arg.At})
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("use setup link: %w", err)
		}
		role, err := q.SetUserPassword(ctx, SetUserPasswordParams{PasswordHash: arg.PasswordHash, ID: userID})
		if err != nil {
			return fmt.Errorf("set user password: %w", err)
		}
		if role == roleOwner {
			if err := q.DeleteUserSessions(ctx, userID); err != nil {
				return fmt.Errorf("delete sessions: %w", err)
			}
		}
		set = 1
		return nil
	})
	if err != nil {
		return 0, err
	}
	return set, nil
}
