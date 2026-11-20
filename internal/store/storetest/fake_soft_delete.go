package storetest

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

func (f *Fake) DeleteProduct(_ context.Context, arg store.DeleteParams, by store.ActivityBy) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	i := slices.IndexFunc(f.products, func(p store.Product) bool { return p.ID == arg.ID && !p.DeletedAt.Valid })
	switch {
	case i < 0:
		return store.ErrNotFound
	case f.products[i].Status != "archived":
		return store.ErrProductNotArchived
	}
	entry := store.ProductDeleted(f.products[i], by)
	if err := f.checkActivity(entry); err != nil {
		return err
	}
	f.products[i].DeletedAt = pgtype.Timestamptz{Time: arg.At, Valid: true}
	f.products[i].DeletedBy = pgtype.Int8{Int64: arg.UserID, Valid: true}
	f.record(entry)
	return nil
}

func (f *Fake) DeleteUser(_ context.Context, arg store.DeleteParams, by store.ActivityBy) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	target := f.userRef(arg.ID)
	switch {
	case target == nil:
		return store.ErrNotFound
	case target.Role == "owner":
		return store.ErrTargetIsOwner
	case target.Status != "inactive":
		return store.ErrUserNotInactive
	}
	entry := store.UserDeleted(snapshot(*target), by)
	if err := f.checkActivity(entry); err != nil {
		return err
	}
	target.DeletedAt = pgtype.Timestamptz{Time: arg.At, Valid: true}
	target.DeletedBy = pgtype.Int8{Int64: arg.UserID, Valid: true}
	f.record(entry)
	return nil
}

func (f *Fake) DeleteOrder(_ context.Context, arg store.DeleteOrderParams, by store.ActivityBy) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	order := f.orderByNumber(arg.Number)
	switch {
	case order == nil:
		return store.ErrNotFound
	case order.Status == store.OrderPending:
		return store.ErrOrderPending
	}
	entry := store.OrderEntry(f.detail(*order).ReadOrderRow, store.ActionDeleted, by)
	if err := f.checkActivity(entry); err != nil {
		return err
	}
	order.DeletedAt = pgtype.Timestamptz{Time: arg.At, Valid: true}
	order.DeletedBy = pgtype.Int8{Int64: arg.UserID, Valid: true}
	f.record(entry)
	return nil
}
