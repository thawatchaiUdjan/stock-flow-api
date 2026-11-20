//go:build slow

package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store/storetest"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/testdb"
)

// now is an instant with Postgres's microsecond precision.
var now = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

// products R17, users-and-access R54, orders R20: whatever code is written, the schema
// refuses a Deleted Product that is Active, a Deleted User who is Active or the Owner, a
// Deleted Order that is Pending, and a mark without an author.
func TestTheSchemaRefusesDeletingWhatMayNotBeDeleted(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	owner := addUser(t, pool, store.User{Name: "Malee", Email: "malee@example.com", Role: "owner", Status: "active"})
	staff := addUser(t, pool, store.User{Name: "Somchai", Email: "somchai@example.com", Role: "staff", Status: "active"})
	if _, err := pool.Exec(ctx, `INSERT INTO products (name, sku, unit, price) VALUES ('Thai tea', 'TEA-01', 'box', 100)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO orders (total, created_by, created_at) VALUES (0, $1, now())`, owner.ID); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name, statement, constraint string
	}{
		{"an Active Product", `UPDATE products SET deleted_at = now(), deleted_by = $1`, "products_deleted_check"},
		{"a Product with no author", `UPDATE products SET status = 'archived', deleted_at = now() WHERE $1::bigint > 0`, "products_deleted_check"},
		{"an Active User", `UPDATE users SET deleted_at = now(), deleted_by = $1 WHERE role = 'staff'`, "users_deleted_check"},
		{"the Owner", `UPDATE users SET status = 'inactive', deleted_at = now(), deleted_by = $1 WHERE role = 'owner'`, "users_deleted_check"},
		{"a Pending Order", `UPDATE orders SET deleted_at = now(), deleted_by = $1`, "orders_deleted_check"},
		{"an Order with no author", `UPDATE orders SET status = 'cancelled', cancelled_by = $1, cancelled_at = now(), deleted_at = now()`, "orders_deleted_check"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := pool.Exec(ctx, c.statement, staff.ID)
			assertViolates(t, err, c.constraint)
		})
	}
}

// users-and-access R54, R16: a Delete and an Activate of one Inactive User at the same
// moment run one after the other. Whichever goes first, the User ends Deleted and never
// Active, or Active and not Deleted, and the other is refused with its own answer.
func TestADeleteAndAnActivateOfOneUserAtOnceRunOneAfterTheOther(t *testing.T) {
	ctx := context.Background()
	for round := range 10 {
		pool := open(t, testdb.URL(t))
		s := store.NewStore(pool)
		owner := addUser(t, pool, store.User{Name: "Malee", Email: "malee@example.com", Role: "owner", Status: "active"})
		target := addUser(t, pool, store.User{Name: "Somchai", Email: "somchai@example.com", Role: "staff", Status: "inactive"})

		var deleted, activated error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			deleted = s.DeleteUser(ctx, store.DeleteParams{ID: target.ID, UserID: owner.ID, At: now}, storetest.SystemBy)
		}()
		go func() {
			defer wg.Done()
			_, activated = s.UpdateUser(ctx, store.ChangeUserParams{ID: target.ID, Status: "active"}, storetest.SystemBy)
		}()
		wg.Wait()

		switch {
		case deleted == nil && errors.Is(activated, store.ErrNotFound): // deleted first
		case errors.Is(deleted, store.ErrUserNotInactive) && activated == nil: // activated first
		default:
			t.Fatalf("round %d: delete err = %v, activate err = %v; want one to win and the other to be refused", round, deleted, activated)
		}
	}
}

// products R17: a Delete and an Activate of one Archived Product at the same moment run one
// after the other, the same way.
func TestADeleteAndAnActivateOfOneProductAtOnceRunOneAfterTheOther(t *testing.T) {
	ctx := context.Background()
	for round := range 10 {
		pool := open(t, testdb.URL(t))
		s := store.NewStore(pool)
		owner := addUser(t, pool, store.User{Name: "Malee", Email: "malee@example.com", Role: "owner", Status: "active"})
		tea, err := s.CreateProduct(ctx, store.InsertProductParams{Name: "Thai tea", Sku: "TEA-01", Unit: "box", Price: 100}, storetest.SystemBy)
		if err != nil {
			t.Fatal(err)
		}
		archived := pgtype.Text{String: "archived", Valid: true}
		if _, err := s.UpdateProduct(ctx, store.ChangeProductParams{ID: tea.ID, Status: archived}, storetest.SystemBy); err != nil {
			t.Fatal(err)
		}

		var deleted, activated error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			deleted = s.DeleteProduct(ctx, store.DeleteParams{ID: tea.ID, UserID: owner.ID, At: now}, storetest.SystemBy)
		}()
		go func() {
			defer wg.Done()
			_, activated = s.UpdateProduct(ctx, store.ChangeProductParams{ID: tea.ID, Status: pgtype.Text{String: "active", Valid: true}}, storetest.SystemBy)
		}()
		wg.Wait()

		switch {
		case deleted == nil && errors.Is(activated, store.ErrNotFound): // deleted first
		case errors.Is(deleted, store.ErrProductNotArchived) && activated == nil: // activated first
		default:
			t.Fatalf("round %d: delete err = %v, activate err = %v; want one to win and the other to be refused", round, deleted, activated)
		}
	}
}

// orders R20, R13: a Delete and a Cancel of one Completed-or-Pending Order at the same
// moment never delete a Pending Order or cancel a Deleted one.
func TestADeleteAndACancelOfOnePendingOrderAtOnceNeverDeleteAPendingOrder(t *testing.T) {
	ctx := context.Background()
	for round := range 10 {
		pool := open(t, testdb.URL(t))
		s := store.NewStore(pool)
		owner := addUser(t, pool, store.User{Name: "Malee", Email: "malee@example.com", Role: "owner", Status: "active"})
		tea := addProduct(t, pool, store.Product{Name: "Thai tea", Sku: "TEA-01", Unit: "box", Price: 100, Stock: 10, Status: "active"})
		created, err := s.CreateOrder(ctx, store.CreateOrderParams{
			UserID: owner.ID, Now: func() time.Time { return now }, Lines: []store.OrderLineParams{{ProductID: tea.ID, Quantity: 1}},
		}, storetest.SystemBy)
		if err != nil {
			t.Fatal(err)
		}

		var deleted, cancelled error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			deleted = s.DeleteOrder(ctx, store.DeleteOrderParams{Number: created.Number, UserID: owner.ID, At: now}, storetest.SystemBy)
		}()
		go func() {
			defer wg.Done()
			_, cancelled = s.CancelOrder(ctx, store.FinishOrderParams{Number: created.Number, UserID: owner.ID, Now: func() time.Time { return now }}, storetest.SystemBy)
		}()
		wg.Wait()

		switch {
		case errors.Is(deleted, store.ErrOrderPending) && cancelled == nil: // deleted too early: refused
		case deleted == nil && cancelled == nil: // cancelled first, then deleted
		default:
			t.Fatalf("round %d: delete err = %v, cancel err = %v", round, deleted, cancelled)
		}
	}
}
