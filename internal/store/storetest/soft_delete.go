package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// runSoftDelete is the part of the contract that is deleting Products, Users and Orders
// (products R17, users-and-access R54, orders R20, business ADR 0013).
func runSoftDelete(t *testing.T, newSubject func(t *testing.T) Subject) {
	ctx := context.Background()
	archive := func(t *testing.T, s Subject, id int64) {
		t.Helper()
		if _, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: id, Status: text("archived")}, SystemBy); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("a deleted Product is in no list and no lookup, cannot change, and frees its SKU", func(t *testing.T) {
		s := newSubject(t)
		by := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, stocked("Thai tea", "TEA-01", 10))
		archive(t, s, tea.ID)

		if err := s.Store.DeleteProduct(ctx, store.DeleteParams{ID: tea.ID, UserID: by.ID, At: at}, SystemBy); err != nil {
			t.Fatal(err)
		}

		if _, err := s.Store.GetProduct(ctx, tea.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("get: err = %v, want ErrNotFound", err)
		}
		if got := listProducts(t, s, store.ListProductsParams{Status: "archived"}); len(got) != 0 {
			t.Errorf("archived list = %v, want none", names(got))
		}
		count, err := s.Store.CountProducts(ctx, store.CountProductsParams{Status: "archived"})
		if err != nil || count != 0 {
			t.Errorf("count = %d, %v; want 0", count, err)
		}
		change := store.ChangeProductParams{ID: tea.ID, Status: text("active")}
		if _, err := s.Store.UpdateProduct(ctx, change, SystemBy); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("update: err = %v, want ErrNotFound", err)
		}
		if err := s.Store.DeleteProduct(ctx, store.DeleteParams{ID: tea.ID, UserID: by.ID, At: at}, SystemBy); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("delete again: err = %v, want ErrNotFound", err)
		}
		if _, err := s.Store.CreateProduct(ctx, newProduct("Other tea", "tea-01"), SystemBy); err != nil {
			t.Errorf("a new Product with the freed SKU: err = %v, want none", err)
		}
		if _, err := s.Store.CreateProduct(ctx, newProduct("Third tea", "TEA-01"), SystemBy); !errors.Is(err, store.ErrSKUTaken) {
			t.Errorf("the SKU in use again: err = %v, want ErrSKUTaken", err)
		}
	})

	t.Run("an Active or unknown Product is not deleted", func(t *testing.T) {
		s := newSubject(t)
		by := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, stocked("Thai tea", "TEA-01", 10))

		if err := s.Store.DeleteProduct(ctx, store.DeleteParams{ID: tea.ID, UserID: by.ID, At: at}, SystemBy); !errors.Is(err, store.ErrProductNotArchived) {
			t.Errorf("Active: err = %v, want ErrProductNotArchived", err)
		}
		if err := s.Store.DeleteProduct(ctx, store.DeleteParams{ID: 404, UserID: by.ID, At: at}, SystemBy); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("unknown: err = %v, want ErrNotFound", err)
		}
		if got, err := s.Store.GetProduct(ctx, tea.ID); err != nil || got.Status != "active" {
			t.Errorf("get = %+v, %v; want it Active", got, err)
		}
	})

	t.Run("a deleted User is in no list and no lookup, cannot change, and frees their email", func(t *testing.T) {
		s := newSubject(t)
		owner := s.AddUser(t, store.User{Name: "Malee", Email: "malee@example.com", Role: "owner", Status: "active"})
		gone := s.AddUser(t, store.User{Name: "Somchai", Email: "somchai@example.com", Role: "staff", Status: "inactive"})

		if err := s.Store.DeleteUser(ctx, store.DeleteParams{ID: gone.ID, UserID: owner.ID, At: at}, SystemBy); err != nil {
			t.Fatal(err)
		}

		if _, err := s.Store.GetUserByEmail(ctx, "SOMCHAI@example.com"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("by email: err = %v, want ErrNotFound", err)
		}
		rows, err := s.Store.ListUsers(ctx)
		if err != nil || len(rows) != 1 || rows[0].ID != owner.ID {
			t.Errorf("users = %+v, %v; want only the Owner", rows, err)
		}
		if _, err := s.Store.UpdateUser(ctx, store.ChangeUserParams{ID: gone.ID, Status: "active"}, SystemBy); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("update: err = %v, want ErrNotFound", err)
		}
		if err := s.Store.DeleteUser(ctx, store.DeleteParams{ID: gone.ID, UserID: owner.ID, At: at}, SystemBy); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("delete again: err = %v, want ErrNotFound", err)
		}
		created, err := s.Store.CreateUser(ctx, store.CreateUserParams{Name: "Somsri", Email: "Somchai@Example.com", Role: "staff", TokenHash: []byte("t"), ExpiresAt: at}, SystemBy)
		if err != nil || created.ID == gone.ID {
			t.Errorf("a new User with the freed email: %+v, %v; want a new one", created, err)
		}
		if got, err := s.Store.GetUserByEmail(ctx, "somchai@example.com"); err != nil || got.ID != created.ID {
			t.Errorf("by email = %+v, %v; want the new User", got, err)
		}
	})

	t.Run("an Active User and the Owner are not deleted", func(t *testing.T) {
		s := newSubject(t)
		owner := s.AddUser(t, store.User{Name: "Malee", Email: "malee@example.com", Role: "owner", Status: "active"})
		active := s.AddUser(t, staff("somchai@example.com"))

		if err := s.Store.DeleteUser(ctx, store.DeleteParams{ID: active.ID, UserID: owner.ID, At: at}, SystemBy); !errors.Is(err, store.ErrUserNotInactive) {
			t.Errorf("Active: err = %v, want ErrUserNotInactive", err)
		}
		if err := s.Store.DeleteUser(ctx, store.DeleteParams{ID: owner.ID, UserID: owner.ID, At: at}, SystemBy); !errors.Is(err, store.ErrTargetIsOwner) {
			t.Errorf("Owner: err = %v, want ErrTargetIsOwner", err)
		}
		if err := s.Store.DeleteUser(ctx, store.DeleteParams{ID: 404, UserID: owner.ID, At: at}, SystemBy); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("unknown: err = %v, want ErrNotFound", err)
		}
		if rows, err := s.Store.ListUsers(ctx); err != nil || len(rows) != 2 {
			t.Errorf("users = %+v, %v; want both", rows, err)
		}
	})

	t.Run("a deleted Order is in no list, lookup or figure, changes no stock, and keeps its number used", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 100, 10))
		createOrder(t, s, newOrder(user, "", line(tea, 2)))
		createOrder(t, s, newOrder(user, "", line(tea, 3)))
		if _, err := s.Store.CompleteOrder(ctx, finish(1, user), SystemBy); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Store.CancelOrder(ctx, finish(2, user), SystemBy); err != nil {
			t.Fatal(err)
		}
		stock, movements := stockOf(t, s, tea.ID), len(listMovements(t, s, store.ListStockMovementsParams{}))
		for _, m := range listMovements(t, s, store.ListStockMovementsParams{}) {
			if m.OrderDeleted {
				t.Errorf("movement %d of a live Order is flagged deleted", m.ID)
			}
		}
		window := store.SalesByDayParams{FromAt: at.Add(-24 * time.Hour), ToAt: at.Add(24 * time.Hour)}
		if sales, err := s.Store.SalesByDay(ctx, window); err != nil || len(sales) != 1 {
			t.Fatalf("sales before = %+v, %v; want the Completed Order's day", sales, err)
		}

		for _, number := range []int64{1, 2} {
			if err := s.Store.DeleteOrder(ctx, store.DeleteOrderParams{Number: number, UserID: user.ID, At: at}, SystemBy); err != nil {
				t.Fatalf("delete %d: %v", number, err)
			}
			if _, err := s.Store.GetOrder(ctx, number); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("get %d: err = %v, want ErrNotFound", number, err)
			}
			if _, err := s.Store.CompleteOrder(ctx, finish(number, user), SystemBy); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("complete %d: err = %v, want ErrNotFound", number, err)
			}
			if err := s.Store.DeleteOrder(ctx, store.DeleteOrderParams{Number: number, UserID: user.ID, At: at}, SystemBy); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("delete %d again: err = %v, want ErrNotFound", number, err)
			}
		}

		if got := listOrders(t, s, store.ListOrdersParams{RowLimit: 100}); len(got) != 0 {
			t.Errorf("orders = %v, want none", got)
		}
		count, err := s.Store.CountOrders(ctx, store.CountOrdersParams{})
		if err != nil || count != 0 {
			t.Errorf("count = %d, %v; want 0", count, err)
		}
		if sales, err := s.Store.SalesByDay(ctx, window); err != nil || len(sales) != 0 {
			t.Errorf("sales = %+v, %v; want none", sales, err)
		}
		top, err := s.Store.TopSellingProducts(ctx, store.TopSellingProductsParams{FromAt: window.FromAt, ToAt: window.ToAt, RowLimit: 20})
		if err != nil || len(top) != 0 {
			t.Errorf("top products = %+v, %v; want none", top, err)
		}
		if got, n := stockOf(t, s, tea.ID), len(listMovements(t, s, store.ListStockMovementsParams{})); got != stock || n != movements {
			t.Errorf("stock %d and %d movements, want %d and %d unchanged", got, n, stock, movements)
		}
		// stock-movements R13: the movements stay, name the number and say the Order is deleted.
		for _, m := range listMovements(t, s, store.ListStockMovementsParams{}) {
			if m.OrderID.Valid && (!m.OrderDeleted || m.OrderNumber.Int64 < 1) {
				t.Errorf("movement %d: order number %d, deleted %v; want its number and deleted", m.ID, m.OrderNumber.Int64, m.OrderDeleted)
			}
		}
		if next := createOrder(t, s, newOrder(user, "", line(tea, 1))); next.Number != 3 {
			t.Errorf("next number = %d, want 3", next.Number)
		}
	})

	t.Run("a Pending or unknown Order is not deleted", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 100, 10))
		createOrder(t, s, newOrder(user, "", line(tea, 2)))

		if err := s.Store.DeleteOrder(ctx, store.DeleteOrderParams{Number: 1, UserID: user.ID, At: at}, SystemBy); !errors.Is(err, store.ErrOrderPending) {
			t.Errorf("Pending: err = %v, want ErrOrderPending", err)
		}
		if err := s.Store.DeleteOrder(ctx, store.DeleteOrderParams{Number: 404, UserID: user.ID, At: at}, SystemBy); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("unknown: err = %v, want ErrNotFound", err)
		}
		if got, err := s.Store.GetOrder(ctx, 1); err != nil || got.Status != "pending" {
			t.Errorf("get = %+v, %v; want it Pending", got, err)
		}
	})
}
