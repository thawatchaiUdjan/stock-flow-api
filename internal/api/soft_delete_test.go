package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

func (h *harness) deleteProduct(id int64, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodDelete, fmt.Sprintf("/products/%d", id), nil, cookies...)
}

func (h *harness) deleteUser(id int64, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodDelete, fmt.Sprintf("/users/%d", id), nil, cookies...)
}

func (h *harness) deleteOrder(number int64, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodDelete, fmt.Sprintf("/orders/%d", number), nil, cookies...)
}

func (h *harness) archive(id int64, cookie *http.Cookie) {
	h.t.Helper()
	if rec := h.updateProduct(id, map[string]any{"status": "archived"}, cookie); rec.Code != http.StatusOK {
		h.t.Fatalf("archive: status = %d; body %s", rec.Code, rec.Body)
	}
}

func (h *harness) userEmails(cookie *http.Cookie) []string {
	h.t.Helper()
	rec := h.listUsers(cookie)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("list users: status = %d; body %s", rec.Code, rec.Body)
	}
	emails := []string{}
	for _, u := range decode[struct {
		Users []userBody `json:"users"`
	}](h.t, rec).Users {
		emails = append(emails, u.Email)
	}
	return emails
}

// products R17: the Owner and Admins delete an Archived Product, which leaves every list
// and lookup and cannot be changed.
func TestTheOwnerAndAnAdminDeleteAnArchivedProductAndItLeavesEveryListAndLookup(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})
	cup := h.seed(store.Product{Name: "Cup", Sku: "CUP-01", Price: 100, Stock: 10})
	h.archive(tea.ID, s.owner)
	h.archive(cup.ID, s.owner)

	for name, c := range map[string]struct {
		id     int64
		cookie *http.Cookie
	}{"the Owner": {tea.ID, s.owner}, "an Admin": {cup.ID, s.admin}} {
		t.Run(name, func(t *testing.T) {
			if rec := h.deleteProduct(c.id, c.cookie); rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want 204; body %s", rec.Code, rec.Body)
			}
			assertProblem(t, h.getProduct(c.id, s.staff), http.StatusNotFound, "")
			assertProblem(t, h.updateProduct(c.id, map[string]any{"status": "active"}, s.owner), http.StatusNotFound, "")
			assertProblem(t, h.deleteProduct(c.id, s.owner), http.StatusNotFound, "")
		})
	}
	for _, status := range []string{"active", "archived"} {
		if page := h.listProducts("status="+status, s.owner); page.Total != 0 || len(page.Products) != 0 {
			t.Errorf("%s list = %+v, want none", status, page)
		}
	}
}

// products R17: Staff cannot, and the Product stays.
func TestStaffAndVisitorsCannotDeleteAProduct(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})
	h.archive(tea.ID, s.owner)

	assertProblem(t, h.deleteProduct(tea.ID, s.staff), http.StatusForbidden, "")
	assertProblem(t, h.deleteProduct(tea.ID), http.StatusUnauthorized, "")

	if page := h.listProducts("status=archived", s.owner); page.Total != 1 {
		t.Errorf("archived Products = %+v, want the one", page)
	}
}

// products R11, R17: only an Archived Product can be deleted.
func TestAnActiveProductCannotBeDeleted(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})

	assertProblem(t, h.deleteProduct(tea.ID, s.owner), http.StatusConflict, "product-not-archived")

	if got := decode[productBody](t, h.getProduct(tea.ID, s.owner)); got.Status != "active" {
		t.Errorf("status = %q, want active", got.Status)
	}
	assertProblem(t, h.deleteProduct(404, s.owner), http.StatusNotFound, "")
}

// products R3: a Deleted Product's SKU is free, and one still in use stays taken.
func TestADeletedProductsSKUCanBeUsedAgain(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100})
	h.archive(tea.ID, s.owner)
	h.deleteProduct(tea.ID, s.owner)

	rec := h.createProduct(with("sku", "tea-01"), s.owner)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body)
	}
	assertProblem(t, h.createProduct(with("sku", "TEA-01"), s.owner), http.StatusConflict, "sku-taken")
}

// products R17: Orders and Stock movements keep showing a Deleted Product, and a Pending
// Order that names it can still be cancelled, which gives its stock back (orders R12).
func TestADeletedProductStaysOnItsOrdersAndMovementsAndACancelGivesItsStockBack(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})
	order := h.mustCreateOrder(s.staff, lineIn{tea.ID, 4})
	h.archive(tea.ID, s.owner)
	h.deleteProduct(tea.ID, s.owner)

	got := decode[orderBody](t, h.getOrder(order.Number, s.staff))
	if len(got.Lines) != 1 || got.Lines[0].Product.Name != "Thai tea" || got.Lines[0].Product.SKU != "TEA-01" {
		t.Errorf("lines = %+v, want Thai tea TEA-01", got.Lines)
	}
	if rec := h.cancelOrder(order.Number, s.owner); rec.Code != http.StatusOK {
		t.Fatalf("cancel: status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	moves := h.listMovements("", s.owner).Movements
	if len(moves) != 2 || moves[0].Kind != "cancellation" || moves[0].StockAfter != 10 || moves[0].Product.Name != "Thai tea" {
		t.Errorf("movements = %+v, want the Order's and its cancellation, naming Thai tea", moves)
	}
}

// products R12, R17: nothing can be received into, adjusted on or ordered from it.
func TestNothingIsReceivedAdjustedOrOrderedForADeletedProduct(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})
	h.archive(tea.ID, s.owner)
	h.deleteProduct(tea.ID, s.owner)

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"receive": h.receive(map[string]any{"productId": tea.ID, "quantity": 1}, s.staff),
		"adjust":  h.adjust(map[string]any{"productId": tea.ID, "change": 1, "note": "count"}, s.owner),
		"order":   h.createOrder(orderOf(lineIn{tea.ID, 1}), s.staff),
	} {
		if rec.Code < 400 || rec.Code >= 500 {
			t.Errorf("%s: status = %d, want a 4xx refusal; body %s", name, rec.Code, rec.Body)
		}
	}
	if page := h.listMovements("", s.owner); len(page.Movements) != 0 {
		t.Errorf("movements = %+v, want none", page.Movements)
	}
}

// users-and-access R54: the Owner deletes an Inactive User, who leaves the Users list and
// is an unknown email to Login.
func TestTheOwnerDeletesAnInactiveUserWhoLeavesTheListAndLogin(t *testing.T) {
	h := newHarness(t)
	owner, target := h.ownerAnd("somchai@example.com", func(u *store.User) { u.Status = "inactive" })

	if rec := h.deleteUser(target.ID, owner); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body %s", rec.Code, rec.Body)
	}

	if emails := h.userEmails(owner); slices.Contains(emails, "somchai@example.com") {
		t.Errorf("users = %v, want Somchai gone", emails)
	}
	assertProblem(t, h.logIn("somchai@example.com", password), http.StatusUnauthorized, "login-failed")
	if got := h.identify("somchai@example.com"); got.Next != "no-account" {
		t.Errorf("identify = %+v, want no-account", got)
	}
	assertProblem(t, h.updateUser(target.ID, map[string]string{"status": "active"}, owner), http.StatusNotFound, "")
	assertProblem(t, h.resetPassword(target.ID, owner), http.StatusNotFound, "")
	assertProblem(t, h.deleteUser(target.ID, owner), http.StatusNotFound, "")
}

// users-and-access R54: only the Owner, only an Inactive User, never the Owner.
func TestOnlyTheOwnerDeletesAUserAndOnlyAnInactiveOneAndNeverTheOwner(t *testing.T) {
	h := newHarness(t)
	owner, active := h.ownerAnd("somchai@example.com")
	admin := h.loggedInAs("admin")
	ownerUser, _ := h.store.GetUserByEmail(t.Context(), ownerSessionEmail)
	h.updateUser(active.ID, inactive, owner)

	assertProblem(t, h.deleteUser(active.ID, admin), http.StatusForbidden, "")
	assertProblem(t, h.deleteUser(active.ID), http.StatusUnauthorized, "")
	assertProblem(t, h.deleteUser(ownerUser.ID, owner), http.StatusForbidden, "target-is-owner")
	assertProblem(t, h.deleteUser(404, owner), http.StatusNotFound, "")

	h.updateUser(active.ID, map[string]string{"status": "active"}, owner)
	assertProblem(t, h.deleteUser(active.ID, owner), http.StatusConflict, "user-not-inactive")
	if emails := h.userEmails(owner); !slices.Contains(emails, "somchai@example.com") || !slices.Contains(emails, ownerSessionEmail) {
		t.Errorf("users = %v, want the Active User and the Owner kept", emails)
	}
}

// users-and-access R3: a Deleted User's email can be used for a new User.
func TestADeletedUsersEmailCanBeUsedAgain(t *testing.T) {
	h := newHarness(t)
	owner, target := h.ownerAnd("somchai@example.com", func(u *store.User) { u.Status = "inactive" })
	h.deleteUser(target.ID, owner)

	rec := h.createUser(map[string]string{"name": "Somsri", "email": "Somchai@Example.com", "role": "staff"}, owner)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body)
	}
	assertProblem(t, h.createUser(map[string]string{"name": "Other", "email": "somchai@example.com", "role": "staff"}, owner), http.StatusConflict, "email-taken")
	if emails := h.userEmails(owner); len(emails) != 2 {
		t.Errorf("users = %v, want the Owner and the new User", emails)
	}
}

// users-and-access R54: what a Deleted User did stays attributed to them.
func TestWhatADeletedUserDidStaysUnderTheirName(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})
	order := h.mustCreateOrder(s.staff, lineIn{tea.ID, 1})
	h.completeOrder(order.Number, s.staff)
	staff, _ := h.store.GetUserByEmail(t.Context(), "staff@example.com")
	h.updateUser(staff.ID, inactive, s.owner)
	if rec := h.deleteUser(staff.ID, s.owner); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d; body %s", rec.Code, rec.Body)
	}

	got := decode[orderBody](t, h.getOrder(order.Number, s.owner))
	if got.CreatedBy.Name != "Somchai" || got.Completed == nil || got.Completed.By.Name != "Somchai" {
		t.Errorf("order = %+v, want created and completed by Somchai", got)
	}
	if moves := h.listMovements("", s.owner).Movements; len(moves) != 1 || moves[0].User.Name != "Somchai" {
		t.Errorf("movements = %+v, want the one by Somchai", moves)
	}
}

// orders R20: the Owner and Admins delete a Completed or Cancelled Order, which leaves the
// list and lookup, changes no stock, and does not free its number.
func TestTheOwnerAndAnAdminDeleteAFinishedOrderAndItLeavesTheListWithItsStockUnchanged(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})
	h.mustCreateOrder(s.staff, lineIn{tea.ID, 1})
	h.mustCreateOrder(s.staff, lineIn{tea.ID, 2})
	keep := h.mustCreateOrder(s.staff, lineIn{tea.ID, 3})
	h.completeOrder(1, s.staff)
	h.cancelOrder(2, s.owner)
	stock := h.stockOf(tea.ID, s.owner)
	movements := len(h.listMovements("", s.owner).Movements)

	for name, c := range map[string]struct {
		number int64
		cookie *http.Cookie
	}{"a Completed Order by the Owner": {1, s.owner}, "a Cancelled Order by an Admin": {2, s.admin}} {
		t.Run(name, func(t *testing.T) {
			if rec := h.deleteOrder(c.number, c.cookie); rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want 204; body %s", rec.Code, rec.Body)
			}
			assertProblem(t, h.getOrder(c.number, s.staff), http.StatusNotFound, "")
			assertProblem(t, h.completeOrder(c.number, s.staff), http.StatusNotFound, "")
			assertProblem(t, h.cancelOrder(c.number, s.owner), http.StatusNotFound, "")
			assertProblem(t, h.deleteOrder(c.number, s.owner), http.StatusNotFound, "")
		})
	}

	if got := h.orderNumbers("", s.owner); !slices.Equal(got, []int64{keep.Number}) {
		t.Errorf("orders = %v, want only %d", got, keep.Number)
	}
	if page := h.listOrders("status=completed", s.owner); page.Total != 0 {
		t.Errorf("completed orders = %+v, want none", page)
	}
	if got := h.stockOf(tea.ID, s.owner); got != stock {
		t.Errorf("stock = %d, want %d unchanged", got, stock)
	}
	if got := len(h.listMovements("", s.owner).Movements); got != movements {
		t.Errorf("movements = %d, want %d: Stock movements are never deleted", got, movements)
	}
	if next := h.mustCreateOrder(s.staff, lineIn{tea.ID, 1}); next.Number != 4 {
		t.Errorf("next number = %d, want 4: a Deleted Order's number is never reused", next.Number)
	}
}

// orders R20: Staff cannot, and a Pending Order cannot be deleted by anyone.
func TestStaffCannotDeleteAnOrderAndNobodyDeletesAPendingOne(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})
	h.mustCreateOrder(s.staff, lineIn{tea.ID, 1})
	h.mustCreateOrder(s.staff, lineIn{tea.ID, 1})
	h.completeOrder(2, s.staff)

	assertProblem(t, h.deleteOrder(2, s.staff), http.StatusForbidden, "")
	assertProblem(t, h.deleteOrder(2), http.StatusUnauthorized, "")
	assertProblem(t, h.deleteOrder(1, s.owner), http.StatusConflict, "order-pending")
	assertProblem(t, h.deleteOrder(404, s.owner), http.StatusNotFound, "")

	if got := h.orderNumbers("", s.owner); !slices.Equal(got, []int64{2, 1}) {
		t.Errorf("orders = %v, want 2 and 1", got)
	}
	if got := decode[orderBody](t, h.getOrder(1, s.owner)); got.Status != "pending" {
		t.Errorf("order 1 = %q, want pending", got.Status)
	}
}

// dashboard-and-reports R2, R10: a Deleted Order is not Sales, not an Order completed, not
// among the newest Orders, and not in the highest-selling Products.
func TestADeletedOrderIsNotInTheDashboardOrTheSalesReport(t *testing.T) {
	h := newHarness(t)
	s := h.shop() // now: 2026-11-06 16:00 in Bangkok
	tea := h.sale("Thai tea", "TEA-01", 4500)
	cup := h.sale("Cup", "CUP-01", 1000)
	h.completeAt(bangkokTime(11, 6, 9, 0, 0), s.staff, lineIn{tea.ID, 2})
	gone := h.completeAt(bangkokTime(11, 6, 10, 0, 0), s.staff, lineIn{cup.ID, 3})
	if rec := h.deleteOrder(gone.Number, s.owner); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d; body %s", rec.Code, rec.Body)
	}

	dash := h.mustDashboard(s.owner)
	report := h.mustSalesReport("from=2026-11-01&to=2026-11-30", s.owner)

	if dash.Sales != 9000 || dash.OrdersCompleted != 1 || len(dash.NewestOrders) != 1 {
		t.Errorf("dashboard = Sales %d, completed %d, newest %d; want 9000, 1, 1", dash.Sales, dash.OrdersCompleted, len(dash.NewestOrders))
	}
	if d := salesOn(dash.SalesByDay, "2026-11-06"); d.Sales != 9000 || d.OrdersCompleted != 1 {
		t.Errorf("today's chart = %+v, want 9000 from 1 Order", d)
	}
	if report.Sales != 9000 || report.OrdersCompleted != 1 || len(report.TopProducts) != 1 || report.TopProducts[0].Name != "Thai tea" {
		t.Errorf("report = %+v, want only the Thai tea Order", report)
	}
}
