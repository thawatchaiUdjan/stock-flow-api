//go:build slow

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/testdb"
)

// orders R8, R9, stock-movements R7: many Orders for the last few units of a Product at
// once never together take more than its stock, and the stock stays the sum of its
// movements (context ADR 0006, ADR 0007).
func TestOrdersForTheLastUnitsAtOnceNeverTakeStockBelowZero(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	user := addUser(t, pool, store.User{Name: "Somchai", Email: "s@example.com", Role: "staff", Status: "active"})
	tea := addProduct(t, pool, store.Product{Name: "Thai tea", Sku: "TEA-01", Unit: "pcs", Price: 4500, Stock: 7, Status: "active"})
	warm(t, pool)

	const n = 30
	errs := atTheSameTime(n, func(i int) error {
		_, err := s.CreateOrder(ctx, store.CreateOrderParams{
			UserID: user.ID, Now: time.Now, Lines: []store.OrderLineParams{{ProductID: tea.ID, Quantity: int32(1 + i%2)}},
		})
		return err
	})

	var taken int32
	for i, err := range errs {
		var short *store.InsufficientStockError
		switch {
		case err == nil:
			taken += int32(1 + i%2)
		case errors.As(err, &short):
		default:
			t.Errorf("err = %v, want nil or InsufficientStockError", err)
		}
	}
	movements, err := s.ListStockMovements(ctx, store.ListStockMovementsParams{RowLimit: n + 1})
	if err != nil {
		t.Fatal(err)
	}
	stock := int32(7)
	for i := len(movements) - 1; i >= 0; i-- {
		m := movements[i]
		stock += m.Change
		if m.Kind != store.KindOrder || m.StockAfter != stock || m.StockAfter < 0 {
			t.Fatalf("movement %d: %s of %d, stock after %d, want an order leaving %d and never below 0", m.ID, m.Kind, m.Change, m.StockAfter, stock)
		}
	}
	got, err := s.GetProduct(ctx, tea.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stock != stock || got.Stock != 7-taken || got.Stock < 0 {
		t.Errorf("stock = %d, want %d: the start less every Order created, and the sum of the movements %d", got.Stock, 7-taken, stock)
	}
	// Seven units in Orders of one or two: whatever the timing, at most one unit is left.
	if got.Stock > 1 {
		t.Errorf("stock = %d with %d Orders refused, want at most 1 left", got.Stock, n)
	}
}

// Two Orders naming the same two Products in opposite order wait for each other instead
// of deadlocking: CreateOrder takes the Products by id, so it waits for the lower id while
// holding nothing the other wants.
func TestOrdersNamingTwoProductsInOppositeOrderDoNotDeadlock(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	user := addUser(t, pool, store.User{Name: "Somchai", Email: "s@example.com", Role: "staff", Status: "active"})
	first := addProduct(t, pool, store.Product{Name: "Thai tea", Sku: "TEA-01", Unit: "pcs", Price: 100, Stock: 10, Status: "active"})
	second := addProduct(t, pool, store.Product{Name: "Cup", Sku: "CUP-01", Unit: "pcs", Price: 100, Stock: 10, Status: "active"})
	other, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Rollback(ctx)
	q := store.New(other)
	if _, err := q.LockProducts(ctx, []int64{first.ID}); err != nil {
		t.Fatal(err)
	}

	created := make(chan error, 1)
	go func() {
		_, err := s.CreateOrder(ctx, store.CreateOrderParams{UserID: user.ID, Now: time.Now, Lines: []store.OrderLineParams{
			{ProductID: second.ID, Quantity: 1}, {ProductID: first.ID, Quantity: 1},
		}})
		created <- err
	}()
	waitUntilBlocked(t, pool)
	// If CreateOrder held the second Product while waiting for the first, this would deadlock.
	if _, err := q.LockProducts(ctx, []int64{second.ID}); err != nil {
		t.Fatalf("the other transaction's second lock: %v", err)
	}
	if err := other.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err := <-created; err != nil {
		t.Errorf("CreateOrder: %v", err)
	}
}

// orders R13: a Complete and a Cancel of one Order at the same moment, only one wins; the
// other changes nothing, and the stock comes back only if the Cancel won.
func TestACompleteAndACancelAtOnceOnlyOneWins(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	user := addUser(t, pool, store.User{Name: "Somchai", Email: "s@example.com", Role: "admin", Status: "active"})
	const n = 10
	tea := addProduct(t, pool, store.Product{Name: "Thai tea", Sku: "TEA-01", Unit: "pcs", Price: 100, Stock: n, Status: "active"})
	var numbers []int64
	for range n {
		o, err := s.CreateOrder(ctx, store.CreateOrderParams{UserID: user.ID, Now: time.Now, Lines: []store.OrderLineParams{{ProductID: tea.ID, Quantity: 1}}})
		if err != nil {
			t.Fatal(err)
		}
		numbers = append(numbers, o.Number)
	}
	warm(t, pool)

	errs := atTheSameTime(2*n, func(i int) error {
		arg := store.FinishOrderParams{Number: numbers[i/2], UserID: user.ID, Now: time.Now}
		if i%2 == 0 {
			_, err := s.CompleteOrder(ctx, arg)
			return err
		}
		_, err := s.CancelOrder(ctx, arg)
		return err
	})

	cancelled := 0
	for i, number := range numbers {
		completeErr, cancelErr := errs[2*i], errs[2*i+1]
		o, err := s.GetOrder(ctx, number)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case completeErr == nil && errors.Is(cancelErr, store.ErrOrderNotPending):
			if o.Status != store.OrderCompleted || o.CancelledAt.Valid {
				t.Errorf("order %d: the Complete won, but it is %+v", number, o.ReadOrderRow)
			}
		case cancelErr == nil && errors.Is(completeErr, store.ErrOrderNotPending):
			cancelled++
			if o.Status != store.OrderCancelled || o.CompletedAt.Valid {
				t.Errorf("order %d: the Cancel won, but it is %+v", number, o.ReadOrderRow)
			}
		default:
			t.Errorf("order %d: complete %v, cancel %v; want exactly one to win and the other ErrOrderNotPending", number, completeErr, cancelErr)
		}
	}
	got, err := s.GetProduct(ctx, tea.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stock != int32(cancelled) {
		t.Errorf("stock = %d, want %d: one back for each Cancel that won", got.Stock, cancelled)
	}
}

// orders R11, R12: the schema refuses a finished Order without who and when, and a
// Pending one with them.
func TestTheSchemaRefusesAnOrderStatusWithoutItsWhoAndWhen(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	user := addUser(t, pool, store.User{Name: "Somchai", Email: "s@example.com", Role: "staff", Status: "active"})
	tea := addProduct(t, pool, store.Product{Name: "Thai tea", Sku: "TEA-01", Unit: "pcs", Price: 100, Stock: 1, Status: "active"})
	o, err := s.CreateOrder(ctx, store.CreateOrderParams{UserID: user.ID, Now: time.Now, Lines: []store.OrderLineParams{{ProductID: tea.ID, Quantity: 1}}})
	if err != nil {
		t.Fatal(err)
	}

	for name, statement := range map[string]string{
		"completed with no one":    `UPDATE orders SET status = 'completed' WHERE id = $1`,
		"cancelled with no time":   `UPDATE orders SET status = 'cancelled', cancelled_by = created_by WHERE id = $1`,
		"pending with a completer": `UPDATE orders SET completed_by = created_by, completed_at = now() WHERE id = $1`,
		"completed and cancelled": `UPDATE orders SET status = 'completed', completed_by = created_by, completed_at = now(),
			cancelled_by = created_by, cancelled_at = now() WHERE id = $1`,
	} {
		_, err := pool.Exec(ctx, statement, o.ID)
		if err == nil {
			t.Errorf("%s: err = nil, want a violation of orders_finished_check", name)
			continue
		}
		assertViolates(t, err, "orders_finished_check")
	}
}
