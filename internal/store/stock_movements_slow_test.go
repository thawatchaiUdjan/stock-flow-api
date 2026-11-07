//go:build slow

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/testdb"
)

// stock-movements R6, R7: many Receives and Adjustments on one Product at once each apply
// to the stock the others left. None is lost, so the stock is the sum of the movements
// (context ADR 0006), and none takes it below 0.
func TestMovementsOnOneProductAtOnceAreNeitherLostNorTakeStockBelowZero(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	user := addUser(t, pool, store.User{Name: "Somchai", Email: "s@example.com", Role: "admin", Status: "active"})
	tea := addProduct(t, pool, store.Product{Name: "Thai tea", Sku: "TEA-01", Unit: "pcs", Stock: 10, Status: "active"})
	warm(t, pool)

	const n = 40
	errs := atTheSameTime(n, func(i int) error {
		arg := store.MoveStockParams{UserID: user.ID, At: time.Now(), Changes: []store.StockChange{{ProductID: tea.ID}}}
		if i%2 == 0 {
			arg.Kind, arg.Changes[0].Change = store.KindReceive, 3
		} else {
			arg.Kind, arg.Note, arg.Changes[0].Change = store.KindAdjustment, "count", -7
		}
		_, err := s.MoveStock(ctx, arg)
		return err
	})

	refused := 0
	for _, err := range errs {
		var short *store.InsufficientStockError
		switch {
		case err == nil:
		case errors.As(err, &short):
			refused++
		default:
			t.Errorf("err = %v, want nil or InsufficientStockError", err)
		}
	}
	movements, err := s.ListStockMovements(ctx, store.ListStockMovementsParams{RowLimit: n + 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(movements) != n-refused {
		t.Fatalf("%d movements, want %d made and %d refused", len(movements), n-refused, refused)
	}
	// Oldest first, each must start from the stock the one before left.
	stock := int32(10)
	for i := len(movements) - 1; i >= 0; i-- {
		m := movements[i]
		stock += m.Change
		if m.StockAfter != stock || m.StockAfter < 0 {
			t.Fatalf("movement %d: change %d, stock after %d, want %d and never below 0", m.ID, m.Change, m.StockAfter, stock)
		}
	}
	got, err := s.GetProduct(ctx, tea.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stock != stock {
		t.Errorf("stock = %d, want %d: the start plus every movement", got.Stock, stock)
	}
}

// Orders lock several Products at once: two transactions naming the same Products in a
// different order must wait for each other, not deadlock. MoveStock takes them by id, so
// it waits for the lower id while holding nothing the other wants.
func TestMovingSeveralProductsWaitsInsteadOfDeadlocking(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	user := addUser(t, pool, store.User{Name: "Somchai", Email: "s@example.com", Role: "staff", Status: "active"})
	first := addProduct(t, pool, store.Product{Name: "Thai tea", Sku: "TEA-01", Unit: "pcs", Stock: 10, Status: "active"})
	second := addProduct(t, pool, store.Product{Name: "Cup", Sku: "CUP-01", Unit: "pcs", Stock: 10, Status: "active"})
	other, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Rollback(ctx)
	q := store.New(other)
	if _, err := q.LockProducts(ctx, []int64{first.ID}); err != nil {
		t.Fatal(err)
	}

	moved := make(chan error, 1)
	go func() {
		_, err := s.MoveStock(ctx, store.MoveStockParams{Kind: store.KindOrder, UserID: user.ID, At: time.Now(), Changes: []store.StockChange{
			{ProductID: second.ID, Change: -1}, {ProductID: first.ID, Change: -1},
		}})
		moved <- err
	}()
	waitUntilBlocked(t, pool)
	// If MoveStock held the second Product while waiting for the first, this would deadlock.
	if _, err := q.LockProducts(ctx, []int64{second.ID}); err != nil {
		t.Fatalf("the other transaction's second lock: %v", err)
	}
	if err := other.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err := <-moved; err != nil {
		t.Errorf("MoveStock: %v", err)
	}
}

// stock-movements R8: the schema refuses to edit or remove a movement, whoever asks.
func TestTheSchemaRefusesToChangeOrRemoveAStockMovement(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	user := addUser(t, pool, store.User{Name: "Somchai", Email: "s@example.com", Role: "staff", Status: "active"})
	tea := addProduct(t, pool, store.Product{Name: "Thai tea", Sku: "TEA-01", Unit: "pcs", Status: "active"})
	if _, err := s.MoveStock(ctx, store.MoveStockParams{
		Kind: store.KindReceive, UserID: user.ID, At: time.Now(), Changes: []store.StockChange{{ProductID: tea.ID, Change: 4}},
	}); err != nil {
		t.Fatal(err)
	}

	for name, statement := range map[string]string{
		"update":   `UPDATE stock_movements SET change = 400, stock_after = 400`,
		"delete":   `DELETE FROM stock_movements`,
		"truncate": `TRUNCATE stock_movements`,
	} {
		_, err := pool.Exec(ctx, statement)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23001" {
			t.Errorf("%s: err = %v, want a restrict_violation", name, err)
		}
	}
	var change int32
	if err := pool.QueryRow(ctx, `SELECT change FROM stock_movements`).Scan(&change); err != nil || change != 4 {
		t.Errorf("change = %d, %v, want it untouched", change, err)
	}
}
