package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrProductNotArchived refuses to delete a Product that is still Active (products R17).
	ErrProductNotArchived = errors.New("the Product is not Archived")
	// ErrUserNotInactive refuses to delete a User who is still Active (users-and-access R54).
	ErrUserNotInactive = errors.New("the User is not Inactive")
	// ErrOrderPending refuses to delete a Pending Order, which still holds stock (orders R20).
	ErrOrderPending = errors.New("the Order is Pending")
)

// DeleteParams is a Product or a User to delete, by the User deleting it, at At.
type DeleteParams struct {
	ID     int64
	UserID int64
	At     time.Time
}

// DeleteProduct marks an Archived Product Deleted and writes its Activity log entry in
// the same transaction (products R17, activity-log R3, business ADR 0013). It answers
// ErrNotFound for no such Product, Deleted ones included, and ErrProductNotArchived for an
// Active one. The Product is held, so an Archive or restore at the same moment runs before
// or after, never between.
func (s *Store) DeleteProduct(ctx context.Context, arg DeleteParams, by ActivityBy) error {
	return InTx(ctx, s.pool, func(q *Queries) error {
		product, err := q.LockProduct(ctx, arg.ID)
		if err != nil {
			return err // ErrNotFound, or a real failure
		}
		if product.Status != "archived" {
			return ErrProductNotArchived
		}
		if _, err := q.MarkProductDeleted(ctx, MarkProductDeletedParams{ID: arg.ID, UserID: arg.UserID, At: arg.At}); err != nil {
			return fmt.Errorf("mark product deleted: %w", err)
		}
		return q.addActivity(ctx, ProductDeleted(product, by))
	})
}

// DeleteUser marks an Inactive User Deleted (users-and-access R54, business ADR 0013). It
// answers ErrNotFound for no such User, Deleted ones included, ErrTargetIsOwner for the
// Owner and ErrUserNotInactive for an Active one. The User is held, as every change to a
// User is, so a reactivation at the same moment runs before or after, never between.
func (s *Store) DeleteUser(ctx context.Context, arg DeleteParams, by ActivityBy) error {
	return InTx(ctx, s.pool, func(q *Queries) error {
		target, err := lockTarget(ctx, q, arg.ID)
		if err != nil {
			return err
		}
		if target.Status != statusInactive {
			return ErrUserNotInactive
		}
		if err := q.MarkUserDeleted(ctx, MarkUserDeletedParams{ID: arg.ID, UserID: arg.UserID, At: arg.At}); err != nil {
			return fmt.Errorf("mark user deleted: %w", err)
		}
		return q.addActivity(ctx, UserDeleted(UserSnapshot{ID: arg.ID, Name: target.Name, Email: target.Email, Role: target.Role, Status: target.Status}, by))
	})
}

// DeleteOrderParams is the Order of Number to delete, by UserID, at At.
type DeleteOrderParams struct {
	Number int64
	UserID int64
	At     time.Time
}

// DeleteOrder marks a Completed or Cancelled Order Deleted, changing no stock (orders R20,
// business ADR 0013). It answers ErrNotFound for no such Order, Deleted ones included, and
// ErrOrderPending for a Pending one. The Order is held, so a Complete or Cancel at the same
// moment runs before or after.
func (s *Store) DeleteOrder(ctx context.Context, arg DeleteOrderParams, by ActivityBy) error {
	return InTx(ctx, s.pool, func(q *Queries) error {
		order, err := q.LockOrder(ctx, arg.Number)
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock order: %w", err)
		}
		if order.Status == OrderPending {
			return ErrOrderPending
		}
		// Read while it is not Deleted yet: the entry names it as it was (activity-log R11).
		detail, err := q.ReadOrder(ctx, arg.Number)
		if err != nil {
			return fmt.Errorf("read order: %w", err)
		}
		if err := q.MarkOrderDeleted(ctx, MarkOrderDeletedParams{ID: order.ID, UserID: arg.UserID, At: arg.At}); err != nil {
			return fmt.Errorf("mark order deleted: %w", err)
		}
		return q.addActivity(ctx, OrderEntry(detail, ActionDeleted, by))
	})
}
