package storetest

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

func stocked(name, sku string, stock int32) store.Product {
	return store.Product{Name: name, Sku: sku, Unit: "pcs", Price: 4500, Stock: stock, Status: "active"}
}

func stockOf(t *testing.T, s Subject, id int64) int32 {
	t.Helper()
	p, err := s.Store.GetProduct(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return p.Stock
}

func listMovements(t *testing.T, s Subject, arg store.ListStockMovementsParams) []store.ListStockMovementsRow {
	t.Helper()
	if arg.RowLimit == 0 {
		arg.RowLimit = 100
	}
	rows, err := s.Store.ListStockMovements(context.Background(), arg)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func move(kind string, user store.User, note string, changes ...store.StockChange) store.MoveStockParams {
	return store.MoveStockParams{Kind: kind, UserID: user.ID, At: at, Note: note, Changes: changes}
}

func change(p store.Product, by int32) store.StockChange {
	return store.StockChange{ProductID: p.ID, Change: by}
}

func ids(rows []store.ListStockMovementsRow) []int64 {
	got := []int64{}
	for _, row := range rows {
		got = append(got, row.ID)
	}
	return got
}

// runStockMovements is the part of the contract that is Stock movements (stock-movements
// R1-R10, context ADR 0006).
func runStockMovements(t *testing.T, newSubject func(t *testing.T) Subject) {
	ctx := context.Background()

	t.Run("a Receive adds to the stock and records the movement with its Product and User", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, stocked("Thai tea", "TEA-01", 5))

		moved, err := s.Store.MoveStock(ctx, move(store.KindReceive, user, "Delivery 12", change(tea, 12)))

		if err != nil {
			t.Fatal(err)
		}
		want := store.ListStockMovementsRow{
			ID: moved[0].ID, Kind: "receive", ProductID: tea.ID, ProductName: "Thai tea", ProductSku: "TEA-01",
			Change: 12, StockAfter: 17, At: at, UserID: user.ID, UserName: "Somchai", Note: "Delivery 12",
		}
		if len(moved) != 1 || !equalMovement(moved[0], want) {
			t.Fatalf("moved = %+v, want [%+v]", moved, want)
		}
		if got := stockOf(t, s, tea.ID); got != 17 {
			t.Errorf("stock = %d, want 17", got)
		}
		if listed := listMovements(t, s, store.ListStockMovementsParams{}); len(listed) != 1 || !equalMovement(listed[0], want) {
			t.Errorf("listed = %+v, want [%+v]", listed, want)
		}
	})

	t.Run("an Adjustment changes the stock either way", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, stocked("Thai tea", "TEA-01", 5))

		down, err := s.Store.MoveStock(ctx, move(store.KindAdjustment, user, "Two broken", change(tea, -2)))
		if err != nil {
			t.Fatal(err)
		}
		up, err := s.Store.MoveStock(ctx, move(store.KindAdjustment, user, "Found one", change(tea, 1)))
		if err != nil {
			t.Fatal(err)
		}

		if down[0].StockAfter != 3 || up[0].StockAfter != 4 || stockOf(t, s, tea.ID) != 4 {
			t.Errorf("stock after = %d then %d, stock %d; want 3, 4, 4", down[0].StockAfter, up[0].StockAfter, stockOf(t, s, tea.ID))
		}
	})

	t.Run("an Adjustment may take the stock to exactly 0", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, stocked("Thai tea", "TEA-01", 5))

		moved, err := s.Store.MoveStock(ctx, move(store.KindAdjustment, user, "Lost", change(tea, -5)))

		if err != nil || moved[0].StockAfter != 0 || stockOf(t, s, tea.ID) != 0 {
			t.Errorf("moved = %+v, %v; stock %d; want stock 0", moved, err, stockOf(t, s, tea.ID))
		}
	})

	t.Run("refuses a movement that would take stock below 0, saying the stock, and changes nothing", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, stocked("Thai tea", "TEA-01", 18))

		_, err := s.Store.MoveStock(ctx, move(store.KindAdjustment, user, "Count", change(tea, -19)))

		var short *store.InsufficientStockError
		if !errors.As(err, &short) {
			t.Fatalf("err = %v, want an InsufficientStockError", err)
		}
		if want := []store.ProductStock{{ProductID: tea.ID, Stock: 18}}; !slices.Equal(short.Products, want) {
			t.Errorf("products = %+v, want %+v", short.Products, want)
		}
		if got := stockOf(t, s, tea.ID); got != 18 {
			t.Errorf("stock = %d, want 18", got)
		}
		if listed := listMovements(t, s, store.ListStockMovementsParams{}); len(listed) != 0 {
			t.Errorf("listed = %+v, want none", listed)
		}
	})

	t.Run("several Products move in one call, each with its own stock after, in the order given", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, stocked("Thai tea", "TEA-01", 10))
		cup := s.AddProduct(t, stocked("Cup", "CUP-01", 3))

		moved, err := s.Store.MoveStock(ctx, move(store.KindOrder, user, "", change(cup, -1), change(tea, -4)))

		if err != nil {
			t.Fatal(err)
		}
		if len(moved) != 2 || moved[0].ProductID != cup.ID || moved[0].StockAfter != 2 ||
			moved[1].ProductID != tea.ID || moved[1].StockAfter != 6 {
			t.Errorf("moved = %+v, want Cup at 2 then Thai tea at 6", moved)
		}
		if stockOf(t, s, tea.ID) != 6 || stockOf(t, s, cup.ID) != 2 {
			t.Errorf("stock = %d and %d, want 6 and 2", stockOf(t, s, tea.ID), stockOf(t, s, cup.ID))
		}
	})

	t.Run("several Products are refused whole, naming each that falls short", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, stocked("Thai tea", "TEA-01", 10))
		cup := s.AddProduct(t, stocked("Cup", "CUP-01", 3))
		lid := s.AddProduct(t, stocked("Lid", "LID-01", 1))

		_, err := s.Store.MoveStock(ctx, move(store.KindOrder, user, "", change(tea, -4), change(lid, -2), change(cup, -5)))

		var short *store.InsufficientStockError
		if !errors.As(err, &short) {
			t.Fatalf("err = %v, want an InsufficientStockError", err)
		}
		want := []store.ProductStock{{ProductID: cup.ID, Stock: 3}, {ProductID: lid.ID, Stock: 1}}
		slices.SortFunc(want, func(a, b store.ProductStock) int { return int(a.ProductID - b.ProductID) })
		if !slices.Equal(short.Products, want) {
			t.Errorf("products = %+v, want %+v", short.Products, want)
		}
		if stockOf(t, s, tea.ID) != 10 || stockOf(t, s, cup.ID) != 3 || stockOf(t, s, lid.ID) != 1 {
			t.Error("a refused call changed some stock")
		}
	})

	t.Run("refuses an unknown Product and changes nothing", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, stocked("Thai tea", "TEA-01", 10))

		_, err := s.Store.MoveStock(ctx, move(store.KindReceive, user, "", change(tea, 1), store.StockChange{ProductID: 404, Change: 1}))

		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
		if got := stockOf(t, s, tea.ID); got != 10 {
			t.Errorf("stock = %d, want 10", got)
		}
	})

	t.Run("refuses an Archived Product for every kind but Cancellation", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		old := stocked("Old tea", "OLD-01", 10)
		old.Status = "archived"
		old = s.AddProduct(t, old)

		for kind, by := range map[string]int32{store.KindReceive: 1, store.KindAdjustment: -1, store.KindOrder: -1} {
			if _, err := s.Store.MoveStock(ctx, move(kind, user, "why", change(old, by))); !errors.Is(err, store.ErrProductArchived) {
				t.Errorf("%s: err = %v, want ErrProductArchived", kind, err)
			}
		}
		if got := stockOf(t, s, old.ID); got != 10 {
			t.Errorf("stock = %d, want 10", got)
		}
		moved, err := s.Store.MoveStock(ctx, move(store.KindCancellation, user, "", change(old, 2)))
		if err != nil || moved[0].StockAfter != 12 {
			t.Errorf("cancellation: moved = %+v, err = %v, want stock after 12", moved, err)
		}
	})

	t.Run("refuses a movement that would take stock past what it can hold", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, stocked("Thai tea", "TEA-01", math.MaxInt32-1))

		_, err := s.Store.MoveStock(ctx, move(store.KindReceive, user, "", change(tea, 2)))

		if !errors.Is(err, store.ErrStockTooLarge) {
			t.Errorf("err = %v, want ErrStockTooLarge", err)
		}
	})

	// The schema backs the API's checks (stock-movements R2-R5): what it refuses changes
	// nothing.
	t.Run("refuses what no movement can be, and changes nothing", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, stocked("Thai tea", "TEA-01", 10))

		for name, arg := range map[string]store.MoveStockParams{
			"a change of 0":                   move(store.KindAdjustment, user, "why", change(tea, 0)),
			"a Receive that takes stock":      move(store.KindReceive, user, "", change(tea, -1)),
			"a Cancellation that takes stock": move(store.KindCancellation, user, "", change(tea, -1)),
			"an Order that adds stock":        move(store.KindOrder, user, "", change(tea, 1)),
			"an Adjustment with no note":      move(store.KindAdjustment, user, "", change(tea, 1)),
			"an Adjustment with a blank note": move(store.KindAdjustment, user, "  ", change(tea, 1)),
			"a note of more than 200 letters": move(store.KindReceive, user, strings.Repeat("ก", 201), change(tea, 1)),
			"an unknown kind":                 move("stocktake", user, "why", change(tea, 1)),
			"an unknown User":                 move(store.KindReceive, store.User{ID: 404}, "", change(tea, 1)),
			"a valid change before a bad one": move(store.KindAdjustment, user, "why", change(tea, 1), change(tea, 0)),
		} {
			if _, err := s.Store.MoveStock(ctx, arg); err == nil {
				t.Errorf("%s: err = nil, want a refusal", name)
			}
		}
		if got := stockOf(t, s, tea.ID); got != 10 {
			t.Errorf("stock = %d, want 10", got)
		}
		if listed := listMovements(t, s, store.ListStockMovementsParams{}); len(listed) != 0 {
			t.Errorf("listed = %+v, want none", listed)
		}
		if _, err := s.Store.MoveStock(ctx, move(store.KindReceive, user, strings.Repeat("ก", 200), change(tea, 1))); err != nil {
			t.Errorf("a note of 200 letters: %v", err)
		}
	})

	t.Run("lists the newest movements first, a page at a time", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, stocked("Thai tea", "TEA-01", 0))
		var made []int64
		for range 5 {
			moved, err := s.Store.MoveStock(ctx, move(store.KindReceive, user, "", change(tea, 1)))
			if err != nil {
				t.Fatal(err)
			}
			made = append(made, moved[0].ID)
		}

		first := listMovements(t, s, store.ListStockMovementsParams{RowLimit: 2})
		second := listMovements(t, s, store.ListStockMovementsParams{RowLimit: 2, Before: pgtype.Int8{Int64: first[1].ID, Valid: true}})
		third := listMovements(t, s, store.ListStockMovementsParams{RowLimit: 2, Before: pgtype.Int8{Int64: second[1].ID, Valid: true}})

		got := slices.Concat(ids(first), ids(second), ids(third))
		slices.Reverse(made)
		if !slices.Equal(got, made) {
			t.Errorf("ids = %v, want %v", got, made)
		}
	})

	t.Run("filters the movements by Product, by kind and by a range of time", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, stocked("Thai tea", "TEA-01", 50))
		cup := s.AddProduct(t, stocked("Cup", "CUP-01", 50))
		made := map[string]int64{}
		for name, arg := range map[string]store.MoveStockParams{
			"tea receive early": {Kind: store.KindReceive, UserID: user.ID, At: at.Add(-time.Hour), Changes: []store.StockChange{change(tea, 1)}},
			"tea receive":       {Kind: store.KindReceive, UserID: user.ID, At: at, Changes: []store.StockChange{change(tea, 1)}},
			"tea adjustment":    {Kind: store.KindAdjustment, UserID: user.ID, At: at, Note: "why", Changes: []store.StockChange{change(tea, -1)}},
			"cup receive":       {Kind: store.KindReceive, UserID: user.ID, At: at, Changes: []store.StockChange{change(cup, 1)}},
			"tea receive late":  {Kind: store.KindReceive, UserID: user.ID, At: at.Add(time.Hour), Changes: []store.StockChange{change(tea, 1)}},
		} {
			moved, err := s.Store.MoveStock(ctx, arg)
			if err != nil {
				t.Fatal(err)
			}
			made[name] = moved[0].ID
		}
		want := func(names ...string) []int64 {
			var w []int64
			for _, n := range names {
				w = append(w, made[n])
			}
			slices.Sort(w)
			slices.Reverse(w)
			return w
		}
		product := pgtype.Int8{Int64: tea.ID, Valid: true}

		for name, c := range map[string]struct {
			arg  store.ListStockMovementsParams
			want []int64
		}{
			"by Product": {store.ListStockMovementsParams{ProductID: product},
				want("tea receive early", "tea receive", "tea adjustment", "tea receive late")},
			"by kind": {store.ListStockMovementsParams{Kind: text("adjustment")}, want("tea adjustment")},
			"from, inclusive": {store.ListStockMovementsParams{ProductID: product, FromAt: pgtype.Timestamptz{Time: at, Valid: true}},
				want("tea receive", "tea adjustment", "tea receive late")},
			"until, exclusive": {store.ListStockMovementsParams{ProductID: product, UntilAt: pgtype.Timestamptz{Time: at, Valid: true}},
				want("tea receive early")},
			"all together": {store.ListStockMovementsParams{ProductID: product, Kind: text("receive"),
				FromAt: pgtype.Timestamptz{Time: at, Valid: true}, UntilAt: pgtype.Timestamptz{Time: at.Add(time.Hour), Valid: true}},
				want("tea receive")},
		} {
			if got := ids(listMovements(t, s, c.arg)); !slices.Equal(got, c.want) {
				t.Errorf("%s: ids = %v, want %v", name, got, c.want)
			}
		}
	})

	t.Run("a movement shows its Product's and User's names as they are now", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, stocked("Thai tea", "TEA-01", 0))
		if _, err := s.Store.MoveStock(ctx, move(store.KindReceive, user, "", change(tea, 1))); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: tea.ID, Name: text("Green tea"), Sku: text("GRN-01")}); err != nil {
			t.Fatal(err)
		}

		listed := listMovements(t, s, store.ListStockMovementsParams{})

		if listed[0].ProductName != "Green tea" || listed[0].ProductSku != "GRN-01" {
			t.Errorf("listed = %+v, want the Product's current name and SKU", listed[0])
		}
	})
}

// equalMovement compares the instant, not the location, which the database does not keep.
func equalMovement(a, b store.ListStockMovementsRow) bool {
	if !a.At.Equal(b.At) {
		return false
	}
	a.At, b.At = time.Time{}, time.Time{}
	return a == b
}
