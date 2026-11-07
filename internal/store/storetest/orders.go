package storetest

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

func priced(name, sku string, price int64, stock int32) store.Product {
	p := stocked(name, sku, stock)
	p.Price = price
	return p
}

func line(p store.Product, quantity int32) store.OrderLineParams {
	return store.OrderLineParams{ProductID: p.ID, Quantity: quantity}
}

func newOrder(user store.User, customer string, lines ...store.OrderLineParams) store.CreateOrderParams {
	return store.CreateOrderParams{UserID: user.ID, Now: clock(at), Customer: customer, Lines: lines}
}

func finish(number int64, user store.User) store.FinishOrderParams {
	return store.FinishOrderParams{Number: number, UserID: user.ID, Now: clock(at.Add(3600e9))}
}

func createOrder(t *testing.T, s Subject, arg store.CreateOrderParams) store.OrderDetail {
	t.Helper()
	created, err := s.Store.CreateOrder(context.Background(), arg)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func listOrders(t *testing.T, s Subject, arg store.ListOrdersParams) []int64 {
	t.Helper()
	if arg.RowLimit == 0 {
		arg.RowLimit = 100
	}
	rows, err := s.Store.ListOrders(context.Background(), arg)
	if err != nil {
		t.Fatal(err)
	}
	count, err := s.Store.CountOrders(context.Background(), store.CountOrdersParams{Status: arg.Status, Search: arg.Search})
	if err != nil {
		t.Fatal(err)
	}
	if arg.RowOffset == 0 && int64(len(rows)) < int64(arg.RowLimit) && count != int64(len(rows)) {
		t.Errorf("count = %d, but the whole list has %d", count, len(rows))
	}
	numbers := []int64{}
	for _, row := range rows {
		numbers = append(numbers, row.Number)
	}
	return numbers
}

// assertNothingTaken fails unless no Order exists, no movement was made and each Product
// has the stock given.
func assertNothingTaken(t *testing.T, s Subject, stock map[int64]int32) {
	t.Helper()
	if numbers := listOrders(t, s, store.ListOrdersParams{}); len(numbers) != 0 {
		t.Errorf("orders = %v, want none", numbers)
	}
	if listed := listMovements(t, s, store.ListStockMovementsParams{}); len(listed) != 0 {
		t.Errorf("movements = %+v, want none", listed)
	}
	for id, want := range stock {
		if got := stockOf(t, s, id); got != want {
			t.Errorf("product %d: stock = %d, want %d", id, got, want)
		}
	}
}

// runOrders is the part of the contract that is Orders (orders R1-R16, context ADR 0007).
func runOrders(t *testing.T, newSubject func(t *testing.T) Subject) {
	ctx := context.Background()

	t.Run("creating an Order fixes its prices, works out its total, starts it Pending and takes its stock", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 4500, 10))
		cup := s.AddProduct(t, priced("Cup", "CUP-01", 1250, 3))

		created, err := s.Store.CreateOrder(ctx, store.CreateOrderParams{
			UserID: user.ID, Now: clock(at), Customer: "Khun Dao", Note: "Pick up at 5",
			Lines: []store.OrderLineParams{line(cup, 3), line(tea, 2)},
		})

		if err != nil {
			t.Fatal(err)
		}
		// orders R1, R3, R4, R10.
		o := created.ReadOrderRow
		if o.Number != 1 || o.Customer != "Khun Dao" || o.Note != "Pick up at 5" || o.Total != 3*1250+2*4500 ||
			o.Status != "pending" || o.CreatedBy != user.ID || o.CreatedByName != "Somchai" || !o.CreatedAt.Equal(at) ||
			o.CompletedBy.Valid || o.CompletedAt.Valid || o.CancelledBy.Valid || o.CancelledAt.Valid {
			t.Errorf("order = %+v", o)
		}
		want := []store.ListOrderLinesRow{
			{ProductID: cup.ID, ProductName: "Cup", ProductSku: "CUP-01", Quantity: 3, Price: 1250},
			{ProductID: tea.ID, ProductName: "Thai tea", ProductSku: "TEA-01", Quantity: 2, Price: 4500},
		}
		if !slices.Equal(created.Lines, want) {
			t.Errorf("lines = %+v, want %+v", created.Lines, want)
		}
		got, err := s.Store.GetOrder(ctx, 1)
		if err != nil || got.ReadOrderRow != created.ReadOrderRow || !slices.Equal(got.Lines, want) {
			t.Errorf("get = %+v, %v; want %+v", got, err, created)
		}
		// orders R7: one Order movement per line, linked to the Order.
		if stockOf(t, s, cup.ID) != 0 || stockOf(t, s, tea.ID) != 8 {
			t.Errorf("stock = %d and %d, want 0 and 8", stockOf(t, s, cup.ID), stockOf(t, s, tea.ID))
		}
		listed := listMovements(t, s, store.ListStockMovementsParams{})
		link := pgtype.Int8{Int64: o.ID, Valid: true}
		number := pgtype.Int8{Int64: 1, Valid: true}
		if len(listed) != 2 {
			t.Fatalf("movements = %+v, want 2", listed)
		}
		for i, wantMove := range []struct {
			product int64
			change  int32
			after   int32
		}{{tea.ID, -2, 8}, {cup.ID, -3, 0}} {
			m := listed[i]
			if m.Kind != "order" || m.ProductID != wantMove.product || m.Change != wantMove.change || m.StockAfter != wantMove.after ||
				m.UserID != user.ID || !m.At.Equal(at) || m.OrderID != link || m.OrderNumber != number {
				t.Errorf("movement %d = %+v, want an order movement of %+v linked to Order 1", i, m, wantMove)
			}
		}
	})

	t.Run("an Order keeps the price it was created with", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 4500, 10))
		createOrder(t, s, newOrder(user, "", line(tea, 2)))

		if _, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: tea.ID, Price: pgtype.Int8{Int64: 9900, Valid: true}}); err != nil {
			t.Fatal(err)
		}

		got, err := s.Store.GetOrder(ctx, 1)
		if err != nil || got.Lines[0].Price != 4500 || got.Total != 9000 {
			t.Errorf("order = %+v, %v; want the price 4500 and total 9000 it was created with", got, err)
		}
	})

	t.Run("Order numbers count up and a refused Order's number is never used", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 100, 10))

		first := createOrder(t, s, newOrder(user, "", line(tea, 1)))
		if _, err := s.Store.CreateOrder(ctx, newOrder(user, "", line(tea, 100))); err == nil {
			t.Fatal("an Order for 100 of 9 was not refused")
		}
		second := createOrder(t, s, newOrder(user, "", line(tea, 1)))
		third := createOrder(t, s, newOrder(user, "", line(tea, 1)))

		if first.Number != 1 || second.Number <= first.Number || third.Number <= second.Number {
			t.Errorf("numbers = %d, %d, %d; want them counting up from 1", first.Number, second.Number, third.Number)
		}
	})

	t.Run("an Order asking for more than the stock is refused whole, naming each short line", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 100, 10))
		cup := s.AddProduct(t, priced("Cup", "CUP-01", 100, 18))
		lid := s.AddProduct(t, priced("Lid", "LID-01", 100, 0))

		_, err := s.Store.CreateOrder(ctx, newOrder(user, "", line(tea, 4), line(lid, 1), line(cup, 19)))

		var short *store.InsufficientStockError
		if !errors.As(err, &short) {
			t.Fatalf("err = %v, want an InsufficientStockError", err)
		}
		want := []store.ProductStock{{ProductID: cup.ID, Stock: 18}, {ProductID: lid.ID, Stock: 0}}
		slices.SortFunc(want, func(a, b store.ProductStock) int { return int(a.ProductID - b.ProductID) })
		if !slices.Equal(short.Products, want) {
			t.Errorf("products = %+v, want %+v", short.Products, want)
		}
		assertNothingTaken(t, s, map[int64]int32{tea.ID: 10, cup.ID: 18, lid.ID: 0})
	})

	t.Run("an Order may take a Product's stock to exactly 0", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 100, 3))

		createOrder(t, s, newOrder(user, "", line(tea, 3)))

		if got := stockOf(t, s, tea.ID); got != 0 {
			t.Errorf("stock = %d, want 0", got)
		}
	})

	t.Run("an Order naming an Archived Product is refused whole", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 100, 10))
		old := priced("Old tea", "OLD-01", 100, 10)
		old.Status = "archived"
		old = s.AddProduct(t, old)

		_, err := s.Store.CreateOrder(ctx, newOrder(user, "", line(tea, 1), line(old, 1)))

		if !errors.Is(err, store.ErrProductArchived) {
			t.Errorf("err = %v, want ErrProductArchived", err)
		}
		assertNothingTaken(t, s, map[int64]int32{tea.ID: 10, old.ID: 10})
	})

	t.Run("an Order naming an unknown Product is refused whole", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 100, 10))

		_, err := s.Store.CreateOrder(ctx, newOrder(user, "", line(tea, 1), store.OrderLineParams{ProductID: 404, Quantity: 1}))

		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
		assertNothingTaken(t, s, map[int64]int32{tea.ID: 10})
	})

	t.Run("an Order whose total is too large to hold is refused", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		var lines []store.OrderLineParams
		stock := map[int64]int32{}
		for _, sku := range []string{"A", "B", "C", "D", "E"} {
			p := s.AddProduct(t, priced("Gold "+sku, "GOLD-"+sku, 999999999, math.MaxInt32))
			lines = append(lines, line(p, math.MaxInt32))
			stock[p.ID] = math.MaxInt32
		}

		_, err := s.Store.CreateOrder(ctx, newOrder(user, "", lines...))

		if !errors.Is(err, store.ErrTotalTooLarge) {
			t.Errorf("err = %v, want ErrTotalTooLarge", err)
		}
		assertNothingTaken(t, s, stock)
	})

	// The schema backs the API's checks (orders R1, R3): what it refuses changes nothing.
	t.Run("refuses what no Order can be, and changes nothing", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 100, 10))
		cup := s.AddProduct(t, priced("Cup", "CUP-01", 100, 10))
		long := func(n int, arg store.CreateOrderParams, set func(*store.CreateOrderParams, string)) store.CreateOrderParams {
			set(&arg, strings.Repeat("ก", n))
			return arg
		}
		ok := newOrder(user, "", line(tea, 1))

		for name, arg := range map[string]store.CreateOrderParams{
			"no lines":                      newOrder(user, ""),
			"a Product on two lines":        newOrder(user, "", line(tea, 1), line(cup, 1), line(tea, 2)),
			"a quantity of 0":               newOrder(user, "", line(cup, 1), line(tea, 0)),
			"a quantity below 0":            newOrder(user, "", line(tea, -1)),
			"a customer of 121 letters":     long(121, ok, func(a *store.CreateOrderParams, v string) { a.Customer = v }),
			"a note of 201 letters":         long(201, ok, func(a *store.CreateOrderParams, v string) { a.Note = v }),
			"an unknown User":               newOrder(store.User{ID: 404}, "", line(tea, 1)),
			"a valid line before a bad one": newOrder(user, "", line(cup, 1), line(tea, 0)),
		} {
			if _, err := s.Store.CreateOrder(ctx, arg); err == nil {
				t.Errorf("%s: err = nil, want a refusal", name)
			}
		}
		assertNothingTaken(t, s, map[int64]int32{tea.ID: 10, cup.ID: 10})
		edge := long(120, long(200, ok, func(a *store.CreateOrderParams, v string) { a.Note = v }), func(a *store.CreateOrderParams, v string) { a.Customer = v })
		if _, err := s.Store.CreateOrder(ctx, edge); err != nil {
			t.Errorf("a customer of 120 letters and a note of 200: %v", err)
		}
	})

	t.Run("completing an Order records who and when, and changes no stock", func(t *testing.T) {
		s := newSubject(t)
		creator := s.AddUser(t, staff("somchai@example.com"))
		other := s.AddUser(t, store.User{Name: "Malee", Email: "malee@example.com", Role: "staff", Status: "active"})
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 100, 10))
		createOrder(t, s, newOrder(creator, "", line(tea, 4)))

		completed, err := s.Store.CompleteOrder(ctx, finish(1, other))

		if err != nil {
			t.Fatal(err)
		}
		o := completed.ReadOrderRow
		if o.Status != "completed" || o.CompletedBy.Int64 != other.ID || o.CompletedByName.String != "Malee" ||
			!o.CompletedAt.Valid || !o.CompletedAt.Time.Equal(at.Add(3600e9)) || o.CancelledBy.Valid || o.CancelledAt.Valid ||
			o.CreatedBy != creator.ID || len(completed.Lines) != 1 {
			t.Errorf("order = %+v", completed)
		}
		if got, err := s.Store.GetOrder(ctx, 1); err != nil || got.ReadOrderRow != o {
			t.Errorf("get = %+v, %v; want %+v", got, err, o)
		}
		if stockOf(t, s, tea.ID) != 6 || len(listMovements(t, s, store.ListStockMovementsParams{})) != 1 {
			t.Errorf("stock = %d, want 6 with only the Order's movement", stockOf(t, s, tea.ID))
		}
	})

	t.Run("cancelling an Order records who and when, and gives each line's stock back", func(t *testing.T) {
		s := newSubject(t)
		creator := s.AddUser(t, staff("somchai@example.com"))
		admin := s.AddUser(t, store.User{Name: "Malee", Email: "malee@example.com", Role: "admin", Status: "active"})
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 100, 10))
		cup := s.AddProduct(t, priced("Cup", "CUP-01", 100, 5))
		created := createOrder(t, s, newOrder(creator, "", line(tea, 4), line(cup, 5)))
		// orders R12: even to a Product Archived since.
		if _, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: cup.ID, Status: text("archived")}); err != nil {
			t.Fatal(err)
		}

		cancelled, err := s.Store.CancelOrder(ctx, finish(1, admin))

		if err != nil {
			t.Fatal(err)
		}
		o := cancelled.ReadOrderRow
		if o.Status != "cancelled" || o.CancelledBy.Int64 != admin.ID || o.CancelledByName.String != "Malee" ||
			!o.CancelledAt.Valid || !o.CancelledAt.Time.Equal(at.Add(3600e9)) || o.CompletedBy.Valid || o.CompletedAt.Valid {
			t.Errorf("order = %+v", o)
		}
		if stockOf(t, s, tea.ID) != 10 || stockOf(t, s, cup.ID) != 5 {
			t.Errorf("stock = %d and %d, want 10 and 5 back", stockOf(t, s, tea.ID), stockOf(t, s, cup.ID))
		}
		listed := listMovements(t, s, store.ListStockMovementsParams{Kind: text("cancellation")})
		link := pgtype.Int8{Int64: created.ID, Valid: true}
		if len(listed) != 2 {
			t.Fatalf("cancellations = %+v, want 2", listed)
		}
		for i, want := range []struct {
			product       int64
			change, after int32
		}{{cup.ID, 5, 5}, {tea.ID, 4, 10}} {
			m := listed[i]
			if m.ProductID != want.product || m.Change != want.change || m.StockAfter != want.after || m.UserID != admin.ID ||
				!m.At.Equal(at.Add(3600e9)) || m.OrderID != link || m.OrderNumber.Int64 != 1 {
				t.Errorf("cancellation %d = %+v, want %+v by the Admin, linked to Order 1", i, m, want)
			}
		}
	})

	t.Run("a Completed or Cancelled Order is final", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 100, 10))
		createOrder(t, s, newOrder(user, "", line(tea, 1)))
		createOrder(t, s, newOrder(user, "", line(tea, 1)))
		if _, err := s.Store.CompleteOrder(ctx, finish(1, user)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Store.CancelOrder(ctx, finish(2, user)); err != nil {
			t.Fatal(err)
		}
		before1, _ := s.Store.GetOrder(ctx, 1)
		before2, _ := s.Store.GetOrder(ctx, 2)

		for name, err := range map[string]error{
			"complete a Completed": second(s.Store.CompleteOrder(ctx, finish(1, user))),
			"cancel a Completed":   second(s.Store.CancelOrder(ctx, finish(1, user))),
			"complete a Cancelled": second(s.Store.CompleteOrder(ctx, finish(2, user))),
			"cancel a Cancelled":   second(s.Store.CancelOrder(ctx, finish(2, user))),
		} {
			if !errors.Is(err, store.ErrOrderNotPending) {
				t.Errorf("%s: err = %v, want ErrOrderNotPending", name, err)
			}
		}
		after1, _ := s.Store.GetOrder(ctx, 1)
		after2, _ := s.Store.GetOrder(ctx, 2)
		if after1.ReadOrderRow != before1.ReadOrderRow || after2.ReadOrderRow != before2.ReadOrderRow {
			t.Errorf("orders changed: %+v, %+v", after1, after2)
		}
		if stockOf(t, s, tea.ID) != 9 || len(listMovements(t, s, store.ListStockMovementsParams{})) != 3 {
			t.Errorf("stock = %d, want 9 after two Orders and one Cancellation", stockOf(t, s, tea.ID))
		}
	})

	t.Run("an unknown Order is not found", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))

		for name, err := range map[string]error{
			"get":      second(s.Store.GetOrder(ctx, 404)),
			"complete": second(s.Store.CompleteOrder(ctx, finish(404, user))),
			"cancel":   second(s.Store.CancelOrder(ctx, finish(404, user))),
		} {
			if !errors.Is(err, store.ErrNotFound) {
				t.Errorf("%s: err = %v, want ErrNotFound", name, err)
			}
		}
	})

	t.Run("lists the newest Orders first, a page at a time, with their line count and total", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 4500, 100))
		cup := s.AddProduct(t, priced("Cup", "CUP-01", 1250, 100))
		for range 3 {
			createOrder(t, s, newOrder(user, "", line(tea, 1)))
		}
		createOrder(t, s, newOrder(user, "Khun Dao", line(tea, 2), line(cup, 1)))

		rows, err := s.Store.ListOrders(ctx, store.ListOrdersParams{RowLimit: 2})
		if err != nil {
			t.Fatal(err)
		}
		want := store.ListOrdersRow{Number: 4, CreatedAt: at, Customer: "Khun Dao", LineCount: 2, Total: 2*4500 + 1250, Status: "pending"}
		if len(rows) != 2 || rows[0].Number != want.Number || !rows[0].CreatedAt.Equal(at) || rows[0].Customer != want.Customer ||
			rows[0].LineCount != want.LineCount || rows[0].Total != want.Total || rows[0].Status != want.Status || rows[1].Number != 3 {
			t.Errorf("first page = %+v, want %+v then Order 3", rows, want)
		}
		if got := listOrders(t, s, store.ListOrdersParams{RowLimit: 2, RowOffset: 2}); !slices.Equal(got, []int64{2, 1}) {
			t.Errorf("second page = %v, want [2 1]", got)
		}
		if count, err := s.Store.CountOrders(ctx, store.CountOrdersParams{}); err != nil || count != 4 {
			t.Errorf("count = %d, %v; want 4", count, err)
		}
	})

	t.Run("searches by number or customer and filters by status", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 100, 100))
		for _, customer := range []string{"Khun Dao", "Somsri", "dao 50%", "Order 1 Ltd", ""} {
			createOrder(t, s, newOrder(user, customer, line(tea, 1)))
		}
		if _, err := s.Store.CompleteOrder(ctx, finish(1, user)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Store.CancelOrder(ctx, finish(2, user)); err != nil {
			t.Fatal(err)
		}

		for name, c := range map[string]struct {
			arg  store.ListOrdersParams
			want []int64
		}{
			"a customer in any letter case": {store.ListOrdersParams{Search: text("DAO")}, []int64{3, 1}},
			"a number, whole":               {store.ListOrdersParams{Search: text("1")}, []int64{4, 1}},
			"a % that means itself":         {store.ListOrdersParams{Search: text("%")}, []int64{3}},
			"Pending":                       {store.ListOrdersParams{Status: text("pending")}, []int64{5, 4, 3}},
			"Completed":                     {store.ListOrdersParams{Status: text("completed")}, []int64{1}},
			"Cancelled":                     {store.ListOrdersParams{Status: text("cancelled")}, []int64{2}},
			"a search and a status":         {store.ListOrdersParams{Search: text("dao"), Status: text("pending")}, []int64{3}},
			"nothing":                       {store.ListOrdersParams{Search: text("nobody")}, []int64{}},
		} {
			if got := listOrders(t, s, c.arg); !slices.Equal(got, c.want) {
				t.Errorf("%s: numbers = %v, want %v", name, got, c.want)
			}
		}
	})
}

// second is the error of a call that also returns a value.
func second[T any](_ T, err error) error { return err }
