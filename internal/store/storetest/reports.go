package storetest

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// completedAt creates an Order of lines and completes it at when.
func completedAt(t *testing.T, s Subject, user store.User, when time.Time, lines ...store.OrderLineParams) store.OrderDetail {
	t.Helper()
	created := createOrder(t, s, newOrder(user, "", lines...))
	done, err := s.Store.CompleteOrder(context.Background(), store.FinishOrderParams{Number: created.Number, UserID: user.ID, Now: clock(when)})
	if err != nil {
		t.Fatal(err)
	}
	return done
}

func salesIn(t *testing.T, s Subject, from, to time.Time) []store.SalesByDayRow {
	t.Helper()
	rows, err := s.Store.SalesByDay(context.Background(), store.SalesByDayParams{FromAt: from, ToAt: to})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func topIn(t *testing.T, s Subject, from, to time.Time, limit int32) []store.TopSellingProductsRow {
	t.Helper()
	rows, err := s.Store.TopSellingProducts(context.Background(), store.TopSellingProductsParams{FromAt: from, ToAt: to, RowLimit: limit})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func productIDs(products []store.Product) []int64 {
	ids := []int64{}
	for _, p := range products {
		ids = append(ids, p.ID)
	}
	return ids
}

// runReports is the part of the contract that is the figures of the Dashboard and Reports
// (dashboard-and-reports R1-R9).
func runReports(t *testing.T, newSubject func(t *testing.T) Subject) {
	// Bangkok midnight at the start of 7 November 2026 is 6 November 17:00 UTC.
	midnight := time.Date(2026, 11, 6, 17, 0, 0, 0, time.UTC)
	day := 24 * time.Hour

	t.Run("sales count only Completed Orders, on the Bangkok day each was completed", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 4500, 100))
		cup := s.AddProduct(t, priced("Cup", "CUP-01", 1000, 100))
		completedAt(t, s, user, midnight.Add(-time.Second), line(tea, 1)) // 6 Nov 23:59:59 in Bangkok
		completedAt(t, s, user, midnight, line(tea, 2))                   // 7 Nov 00:00:00
		completedAt(t, s, user, midnight.Add(day-time.Second), line(tea, 3), line(cup, 1))
		createOrder(t, s, newOrder(user, "", line(tea, 4))) // Pending
		cancelled := createOrder(t, s, newOrder(user, "", line(tea, 5)))
		if _, err := s.Store.CancelOrder(context.Background(), finish(cancelled.Number, user)); err != nil {
			t.Fatal(err)
		}

		got := salesIn(t, s, midnight.Add(-30*day), midnight.Add(30*day))

		// dashboard-and-reports R1, R2.
		want := []store.SalesByDayRow{
			{Day: "2026-11-06", OrdersCompleted: 1, Sales: 4500},
			{Day: "2026-11-07", OrdersCompleted: 2, Sales: 2*4500 + 3*4500 + 1000},
		}
		if !slices.Equal(got, want) {
			t.Errorf("sales = %+v, want %+v", got, want)
		}
	})

	t.Run("sales by day takes the first instant of the range and not the last", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 100, 100))
		completedAt(t, s, user, midnight, line(tea, 1))

		if got := salesIn(t, s, midnight, midnight.Add(day)); len(got) != 1 {
			t.Errorf("sales from the first instant = %+v, want the Order", got)
		}
		if got := salesIn(t, s, midnight.Add(-day), midnight); len(got) != 0 {
			t.Errorf("sales up to the last instant = %+v, want none", got)
		}
	})

	t.Run("sales by day is none when nothing was completed", func(t *testing.T) {
		s := newSubject(t)

		got := salesIn(t, s, midnight.Add(-30*day), midnight.Add(30*day))

		if len(got) != 0 {
			t.Errorf("sales = %+v, want none", got)
		}
	})

	t.Run("top selling Products are ranked by Sales at the prices the Orders were made with", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 4500, 100))
		cup := s.AddProduct(t, priced("Cup", "CUP-01", 1000, 100))
		pen := s.AddProduct(t, priced("Pen", "PEN-01", 500, 100))
		completedAt(t, s, user, midnight, line(tea, 1), line(cup, 6))
		completedAt(t, s, user, midnight.Add(time.Hour), line(cup, 1), line(pen, 3))
		createOrder(t, s, newOrder(user, "", line(pen, 50))) // Pending: not Sales
		// A new price changes no Order already made.
		if _, err := s.Store.UpdateProduct(context.Background(), store.ChangeProductParams{ID: cup.ID, Price: pgtype.Int8{Int64: 2000, Valid: true}}); err != nil {
			t.Fatal(err)
		}
		completedAt(t, s, user, midnight.Add(2*time.Hour), line(cup, 1))

		got := topIn(t, s, midnight, midnight.Add(day), 20)

		// dashboard-and-reports R8: Cup sold 7 at 1000 and 1 at 2000; Thai tea 1 at 4500; Pen 3 at 500.
		want := []store.TopSellingProductsRow{
			{ID: cup.ID, Name: "Cup", Sku: "CUP-01", QuantitySold: 8, Sales: 9000},
			{ID: tea.ID, Name: "Thai tea", Sku: "TEA-01", QuantitySold: 1, Sales: 4500},
			{ID: pen.ID, Name: "Pen", Sku: "PEN-01", QuantitySold: 3, Sales: 1500},
		}
		if !slices.Equal(got, want) {
			t.Errorf("top = %+v, want %+v", got, want)
		}
	})

	t.Run("top selling Products stop at the limit, and a tie goes to the larger quantity and then the name", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		bulk := s.AddProduct(t, priced("Bulk", "BUL-01", 100, 100))
		few := s.AddProduct(t, priced("Few", "FEW-01", 1000, 100))
		apple := s.AddProduct(t, priced("apple", "APP-01", 1000, 100))
		banana := s.AddProduct(t, priced("Banana", "BAN-01", 1000, 100))
		completedAt(t, s, user, midnight, line(bulk, 100), line(few, 10), line(banana, 10), line(apple, 10))

		all := topIn(t, s, midnight, midnight.Add(day), 20)
		two := topIn(t, s, midnight, midnight.Add(day), 2)

		// All four sold for 10000. Bulk sold the most units; the others tie on units, and
		// apple comes before Banana whatever the letter case.
		got := []int64{}
		for _, r := range all {
			got = append(got, r.ID)
		}
		if want := []int64{bulk.ID, apple.ID, banana.ID, few.ID}; !slices.Equal(got, want) {
			t.Errorf("top = %v, want %v", got, want)
		}
		if len(two) != 2 || two[0].ID != bulk.ID || two[1].ID != apple.ID {
			t.Errorf("top 2 = %+v, want the first two", two)
		}
	})

	t.Run("top selling Products include an Archived Product that sold", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 4500, 100))
		completedAt(t, s, user, midnight, line(tea, 1))
		if _, err := s.Store.UpdateProduct(context.Background(), store.ChangeProductParams{ID: tea.ID, Status: pgtype.Text{String: "archived", Valid: true}}); err != nil {
			t.Fatal(err)
		}

		got := topIn(t, s, midnight, midnight.Add(day), 20)

		if len(got) != 1 || got[0].ID != tea.ID {
			t.Errorf("top = %+v, want the Archived Product", got)
		}
	})

	t.Run("top selling Products are none when nothing was completed", func(t *testing.T) {
		s := newSubject(t)

		got := topIn(t, s, midnight, midnight.Add(day), 20)

		if len(got) != 0 {
			t.Errorf("top = %+v, want none", got)
		}
	})

	t.Run("Products to restock are the Active Products Out of stock or Low, the lowest stock first", func(t *testing.T) {
		s := newSubject(t)
		minimum := func(name, sku string, stock, min int32) store.Product {
			p := stocked(name, sku, stock)
			p.MinimumStock = min
			return p
		}
		s.AddProduct(t, minimum("Plenty", "PLE-01", 100, 10))
		atMinimum := s.AddProduct(t, minimum("At minimum", "MIN-01", 10, 10))
		few := s.AddProduct(t, minimum("Few", "FEW-01", 2, 10))
		gone := s.AddProduct(t, minimum("Gone", "GON-01", 0, 10))
		banana := s.AddProduct(t, minimum("Banana", "BAN-01", 0, 0))
		apple := s.AddProduct(t, minimum("apple", "APP-01", 2, 10))
		archived := minimum("Old", "OLD-01", 0, 10)
		archived.Status = "archived"
		s.AddProduct(t, archived)

		got, err := s.Store.ListProductsToRestock(context.Background(), 10)

		if err != nil {
			t.Fatal(err)
		}
		// dashboard-and-reports R5, products R9: Out of stock first, which is stock 0, then
		// the lowest stock, ties by name.
		want := []int64{banana.ID, gone.ID, apple.ID, few.ID, atMinimum.ID}
		if ids := productIDs(got); !slices.Equal(ids, want) {
			t.Errorf("restock = %v, want %v", ids, want)
		}
	})

	t.Run("Products to restock stop at the limit, and are none when none need it", func(t *testing.T) {
		s := newSubject(t)
		none, err := s.Store.ListProductsToRestock(context.Background(), 10)
		if err != nil || len(none) != 0 {
			t.Errorf("restock with no Products = %#v, %v; want none", none, err)
		}
		s.AddProduct(t, stocked("One", "ONE-01", 0))
		s.AddProduct(t, stocked("Two", "TWO-01", 0))

		got, err := s.Store.ListProductsToRestock(context.Background(), 1)

		if err != nil || len(got) != 1 || got[0].Name != "One" {
			t.Errorf("restock = %+v, %v; want only One", got, err)
		}
	})
}
