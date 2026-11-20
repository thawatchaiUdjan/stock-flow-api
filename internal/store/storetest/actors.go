package storetest

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// runActors is the part of the contract that is who acts, how an Order was placed and
// which app a login was for (business ADR 0014: orders R12, R21; stock-movements R2;
// users-and-access R36, R57).
func runActors(t *testing.T, newSubject func(t *testing.T) Subject) {
	ctx := context.Background()

	t.Run("lists Orders by channel, and counts them the same", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 4500, 10))
		inShop := createOrder(t, s, newOrder(user, "Walk-in", line(tea, 1))).Number
		online := s.AddOnlineOrder(t, OnlineOrderSeed{Customer: "Somying", ProductID: tea.ID, Quantity: 2, Price: 4500, At: at})

		for name, tc := range map[string]struct {
			channel string
			want    []int64
		}{
			"both":    {"", []int64{online, inShop}},
			"in-shop": {store.ChannelInShop, []int64{inShop}},
			"online":  {store.ChannelOnline, []int64{online}},
		} {
			channel := pgtype.Text{String: tc.channel, Valid: tc.channel != ""}
			rows, err := s.Store.ListOrders(ctx, store.ListOrdersParams{Channel: channel, RowLimit: 100})
			if err != nil {
				t.Fatal(err)
			}
			var got []int64
			for _, r := range rows {
				got = append(got, r.Number)
				if r.Channel != store.ChannelInShop && r.Channel != store.ChannelOnline {
					t.Errorf("%s: channel = %q", name, r.Channel)
				}
			}
			count, err := s.Store.CountOrders(ctx, store.CountOrdersParams{Channel: channel})
			if err != nil || count != int64(len(tc.want)) || len(got) != len(tc.want) || (len(got) > 0 && got[0] != tc.want[0]) {
				t.Errorf("%s: numbers = %v, count = %d, %v, want %v", name, got, count, err, tc.want)
			}
		}
		got, err := s.Store.GetOrder(ctx, online)
		if err != nil || got.Channel != store.ChannelOnline || got.CreatedBy.Valid || got.Customer != "Somying" || got.Total != 9000 {
			t.Errorf("online order = %+v, %v, want an Online order with no creator User", got, err)
		}
	})

	t.Run("cancels as the system: no User on the Order or its movements", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 4500, 10))
		order := createOrder(t, s, newOrder(user, "", line(tea, 3)))

		cancelled, err := s.Store.CancelOrder(ctx, store.FinishOrderParams{Number: order.Number, Now: clock(at.Add(time.Hour))})
		if err != nil {
			t.Fatal(err)
		}

		if cancelled.Status != store.OrderCancelled || cancelled.CancelledBy.Valid || !cancelled.CancelledBySystem {
			t.Errorf("cancelled = %+v, want cancelled by the system", cancelled.ReadOrderRow)
		}
		byUser, err := s.Store.ListStockMovements(ctx, store.ListStockMovementsParams{InShopOnly: true, RowLimit: 100})
		if err != nil {
			t.Fatal(err)
		}
		all, err := s.Store.ListStockMovements(ctx, store.ListStockMovementsParams{RowLimit: 100})
		if err != nil {
			t.Fatal(err)
		}
		if len(byUser) != 1 || byUser[0].Kind != store.KindOrder || len(all) != 2 {
			t.Fatalf("users only = %+v, all = %+v, want the Order's movement and then both", byUser, all)
		}
		back := all[0]
		if back.Kind != store.KindCancellation || back.UserID.Valid || !back.System || back.StockAfter != 10 {
			t.Errorf("cancellation = %+v, want the system's, stock back to 10", back)
		}
		count, err := s.Store.CountStockMovements(ctx, store.CountStockMovementsParams{InShopOnly: true})
		if err != nil || count != 1 {
			t.Errorf("users-only count = %d, %v, want 1", count, err)
		}
	})

	t.Run("leaves the movements of an Online order out of the legacy list", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 4500, 10))
		createOrder(t, s, newOrder(user, "", line(tea, 1)))
		online := s.AddOnlineOrder(t, OnlineOrderSeed{Customer: "Somying", ProductID: tea.ID, Quantity: 2, Price: 4500, At: at})
		// A User cancels the Online order: their movement links to an Order the legacy operations cannot open.
		if _, err := s.Store.CancelOrder(ctx, store.FinishOrderParams{Number: online, UserID: user.ID, Now: clock(at)}); err != nil {
			t.Fatal(err)
		}

		legacy, err := s.Store.ListStockMovements(ctx, store.ListStockMovementsParams{InShopOnly: true, RowLimit: 100})
		if err != nil {
			t.Fatal(err)
		}
		all, err := s.Store.ListStockMovements(ctx, store.ListStockMovementsParams{RowLimit: 100})
		if err != nil {
			t.Fatal(err)
		}
		count, err := s.Store.CountStockMovements(ctx, store.CountStockMovementsParams{InShopOnly: true})
		if len(legacy) != 1 || legacy[0].Kind != store.KindOrder || len(all) != 2 || err != nil || count != 1 {
			t.Errorf("legacy = %+v (count %d, %v), all = %+v, want only the In-shop order's movement", legacy, count, err, all)
		}
	})

	t.Run("completes only as a User", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 4500, 10))
		order := createOrder(t, s, newOrder(user, "", line(tea, 1)))

		if _, err := s.Store.CompleteOrder(ctx, store.FinishOrderParams{Number: order.Number, Now: clock(at)}); err == nil {
			t.Error("the system completed an Order: err = nil")
		}
		if got, err := s.Store.GetOrder(ctx, order.Number); err != nil || got.Status != store.OrderPending {
			t.Errorf("order = %+v, %v, want it Pending", got, err)
		}
	})

	t.Run("counts every Order in Sales, whatever its channel", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 4500, 10))
		s.AddOnlineOrder(t, OnlineOrderSeed{Customer: "Somying", ProductID: tea.ID, Quantity: 2, Price: 4500, CompletedBy: user.ID, At: at})

		got := salesIn(t, s, at.Add(-24*time.Hour), at.Add(24*time.Hour))

		if len(got) != 1 || got[0].OrdersCompleted != 1 || got[0].Sales != 9000 {
			t.Errorf("sales = %+v, want the Online order's 9000", got)
		}
	})

	t.Run("keeps each app's logins apart", func(t *testing.T) {
		s := newSubject(t)
		entryOf := func(app pgtype.Text, result string) store.AddLoginLogEntryParams {
			return store.AddLoginLogEntryParams{At: at, Email: "somying@example.com", Result: result, Ip: "203.0.113.7", UserAgent: "Firefox", App: app}
		}
		for _, e := range []store.AddLoginLogEntryParams{
			entryOf(pgtype.Text{}, store.ResultLogin), // no app: StockFlow's
			entryOf(text(store.AppStorefront), store.ResultFailedLogin),
		} {
			if err := s.Store.AddLoginLogEntry(ctx, e); err != nil {
				t.Fatal(err)
			}
		}

		for app, want := range map[string]int{"": 2, store.AppStockFlow: 1, store.AppStorefront: 1} {
			filter := pgtype.Text{String: app, Valid: app != ""}
			rows := listLog(t, s, store.ListLoginLogParams{App: filter})
			count, err := s.Store.CountLoginLog(ctx, store.CountLoginLogParams{App: filter})
			if err != nil || len(rows) != want || count != int64(want) {
				t.Errorf("app %q: %d rows, count %d, %v, want %d", app, len(rows), count, err, want)
			}
		}
		if rows := listLog(t, s, store.ListLoginLogParams{App: text(store.AppStockFlow)}); rows[0].App != store.AppStockFlow || rows[0].CustomerID.Valid {
			t.Errorf("StockFlow entry = %+v", rows[0])
		}
	})

	t.Run("counts failed logins per app", func(t *testing.T) {
		s := newSubject(t)
		for i := range store.EmailFailureLimit {
			e := store.AddLoginLogEntryParams{
				At: at.Add(time.Duration(i) * time.Second), Email: "somying@example.com", Result: store.ResultFailedLogin,
				Ip: "203.0.113.7", UserAgent: "Firefox", App: text(store.AppStorefront),
			}
			if err := s.Store.AddLoginLogEntry(ctx, e); err != nil {
				t.Fatal(err)
			}
		}
		blocked := func(app string) bool {
			until, err := s.Store.LoginBlockedUntil(ctx, store.LoginBlockParams{App: app, Email: "somying@example.com", IP: "203.0.113.7", At: at.Add(time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			return !until.IsZero()
		}

		if !blocked(store.AppStorefront) || blocked(store.AppStockFlow) {
			t.Errorf("blocked: Storefront %v, StockFlow %v, want true and false", blocked(store.AppStorefront), blocked(store.AppStockFlow))
		}
	})
}
