package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrTargetIsOwner refuses a change to the Owner's User (users-and-access R18).
	ErrTargetIsOwner = errors.New("the target is the Owner")
	// ErrUserInactive refuses a Setup link for an Inactive User (users-and-access R32).
	ErrUserInactive = errors.New("the User is Inactive")
)

const (
	roleOwner      = "owner"
	statusInactive = "inactive"
)

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

// UpdateUser changes what arg names, in one transaction: an Inactive User has no session
// and no Setup link left (users-and-access R16, R32). It answers ErrNotFound for no such
// User and ErrTargetIsOwner for the Owner.
func (s *Store) UpdateUser(ctx context.Context, arg ChangeUserParams) (updated ChangeUserRow, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		target, err := q.LockUser(ctx, arg.ID)
		if err != nil {
			return err
		}
		if target.Role == roleOwner {
			return ErrTargetIsOwner
		}
		updated, err = q.ChangeUser(ctx, arg)
		if err != nil || updated.Status != statusInactive {
			return err
		}
		if err := q.DeleteUserSessions(ctx, arg.ID); err != nil {
			return err
		}
		return q.DeleteSetupLink(ctx, arg.ID)
	})
	return updated, err
}

// ResetPassword ends the User's password and sessions and issues arg in place of any
// earlier Setup link, in one transaction (users-and-access R29, R31). It answers
// ErrNotFound for no such User, ErrTargetIsOwner for the Owner (R34) and ErrUserInactive
// for an Inactive User (R32), and then changes nothing.
func (s *Store) ResetPassword(ctx context.Context, arg IssueSetupLinkParams) error {
	return InTx(ctx, s.pool, func(q *Queries) error {
		target, err := q.LockUser(ctx, arg.UserID)
		if err != nil {
			return err
		}
		switch {
		case target.Role == roleOwner:
			return ErrTargetIsOwner
		case target.Status == statusInactive:
			return ErrUserInactive
		}
		if err := q.ClearPassword(ctx, arg.UserID); err != nil {
			return err
		}
		if err := q.DeleteUserSessions(ctx, arg.UserID); err != nil {
			return err
		}
		return q.IssueSetupLink(ctx, arg)
	})
}
