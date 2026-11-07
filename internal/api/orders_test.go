package api_test

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

type orderUserBody struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type orderLineBody struct {
	Product struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		SKU  string `json:"sku"`
	} `json:"product"`
	Quantity int64 `json:"quantity"`
	Price    int64 `json:"price"`
	Amount   int64 `json:"amount"`
}

type orderFinishBody struct {
	At time.Time     `json:"at"`
	By orderUserBody `json:"by"`
}

type orderBody struct {
	ID        int64            `json:"id"`
	Number    int64            `json:"number"`
	CreatedAt time.Time        `json:"createdAt"`
	CreatedBy orderUserBody    `json:"createdBy"`
	Customer  string           `json:"customer"`
	Note      string           `json:"note"`
	Lines     []orderLineBody  `json:"lines"`
	Total     int64            `json:"total"`
	Status    string           `json:"status"`
	Completed *orderFinishBody `json:"completed"`
	Cancelled *orderFinishBody `json:"cancelled"`
}

type orderListItem struct {
	ID        int64     `json:"id"`
	Number    int64     `json:"number"`
	CreatedAt time.Time `json:"createdAt"`
	Customer  string    `json:"customer"`
	LineCount int64     `json:"lineCount"`
	Total     int64     `json:"total"`
	Status    string    `json:"status"`
}

type orderPage struct {
	Orders []orderListItem `json:"orders"`
	Total  int64           `json:"total"`
}

type lineIn struct {
	ProductID int64 `json:"productId"`
	Quantity  int64 `json:"quantity"`
}

func orderOf(lines ...lineIn) map[string]any {
	return map[string]any{"lines": lines}
}

func (h *harness) createOrder(body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, "/orders", body, cookies...)
}

// mustCreateOrder creates an Order and returns it.
func (h *harness) mustCreateOrder(cookie *http.Cookie, lines ...lineIn) orderBody {
	h.t.Helper()
	rec := h.createOrder(orderOf(lines...), cookie)
	if rec.Code != http.StatusCreated {
		h.t.Fatalf("create order: status = %d, want 201; body %s", rec.Code, rec.Body)
	}
	return decode[orderBody](h.t, rec)
}

func (h *harness) getOrder(number int64, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodGet, fmt.Sprintf("/orders/%d", number), nil, cookies...)
}

func (h *harness) completeOrder(number int64, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, fmt.Sprintf("/orders/%d/complete", number), nil, cookies...)
}

func (h *harness) cancelOrder(number int64, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, fmt.Sprintf("/orders/%d/cancel", number), nil, cookies...)
}

func (h *harness) listOrders(query string, cookies ...*http.Cookie) orderPage {
	h.t.Helper()
	rec := h.do(http.MethodGet, "/orders?"+query, nil, cookies...)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("list orders %q: status = %d, want 200; body %s", query, rec.Code, rec.Body)
	}
	return decode[orderPage](h.t, rec)
}

func (h *harness) orderNumbers(query string, cookie *http.Cookie) []int64 {
	h.t.Helper()
	numbers := []int64{}
	for _, o := range h.listOrders(query, cookie).Orders {
		numbers = append(numbers, o.Number)
	}
	return numbers
}

// assertNothingTaken fails unless there is no Order, no Stock movement, and each Product
// has the stock given.
func (h *harness) assertNothingTaken(cookie *http.Cookie, stock map[int64]int64) {
	h.t.Helper()
	if page := h.listOrders("", cookie); len(page.Orders) != 0 || page.Total != 0 {
		h.t.Errorf("orders = %+v, want none", page)
	}
	if page := h.listMovements("", cookie); len(page.Movements) != 0 {
		h.t.Errorf("movements = %+v, want none", page.Movements)
	}
	for id, want := range stock {
		if got := h.stockOf(id, cookie); got != want {
			h.t.Errorf("product %d: stock = %d, want %d", id, got, want)
		}
	}
}

// orders R1, R3, R4, R5, R7, R10: Staff create an Order; each line's price is the
// Product's, its amount is quantity times price, the total is their sum in satang, it
// starts Pending, and each line's quantity leaves its Product's stock.
func TestStaffCreateAnOrderThatFixesItsPricesAndTakesItsStock(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 4500, Stock: 10})
	cup := h.seed(store.Product{Name: "Cup", Sku: "CUP-01", Price: 1250, Stock: 3})

	rec := h.createOrder(map[string]any{
		"customer": "Khun Dao", "note": "Pick up at 5",
		"lines": []lineIn{{cup.ID, 3}, {tea.ID, 2}},
	}, s.staff)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body)
	}
	got := decode[orderBody](t, rec)
	if got.ID == 0 || got.Number != 1 || got.Customer != "Khun Dao" || got.Note != "Pick up at 5" ||
		got.Total != 3750+9000 || got.Status != "pending" || got.CreatedBy != (orderUserBody{3, "Somchai"}) ||
		!got.CreatedAt.Equal(h.clock.Now()) || got.Completed != nil || got.Cancelled != nil {
		t.Errorf("order = %+v", got)
	}
	if len(got.Lines) != 2 {
		t.Fatalf("lines = %+v, want 2", got.Lines)
	}
	for i, want := range []struct {
		id                      int64
		name, sku               string
		quantity, price, amount int64
	}{{cup.ID, "Cup", "CUP-01", 3, 1250, 3750}, {tea.ID, "Thai tea", "TEA-01", 2, 4500, 9000}} {
		l := got.Lines[i]
		if l.Product.ID != want.id || l.Product.Name != want.name || l.Product.SKU != want.sku ||
			l.Quantity != want.quantity || l.Price != want.price || l.Amount != want.amount {
			t.Errorf("line %d = %+v, want %+v", i, l, want)
		}
	}
	for _, null := range []string{`"completed":null`, `"cancelled":null`} {
		if !strings.Contains(rec.Body.String(), null) {
			t.Errorf("body %s, want %s", rec.Body, null)
		}
	}
	if h.stockOf(cup.ID, s.staff) != 0 || h.stockOf(tea.ID, s.staff) != 8 {
		t.Errorf("stock = %d and %d, want 0 and 8", h.stockOf(cup.ID, s.staff), h.stockOf(tea.ID, s.staff))
	}
	// orders R17: opening it shows the same.
	if again := decode[orderBody](t, h.getOrder(1, s.staff)); !equalOrders(again, got) {
		t.Errorf("get = %+v, want %+v", again, got)
	}
}

// equalOrders compares the instants, not the locations, which JSON does not keep.
func equalOrders(a, b orderBody) bool {
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return false
	}
	a.CreatedAt, b.CreatedAt = time.Time{}, time.Time{}
	for _, f := range []*[2]*orderFinishBody{{a.Completed, b.Completed}, {a.Cancelled, b.Cancelled}} {
		if (f[0] == nil) != (f[1] == nil) || f[0] != nil && (!f[0].At.Equal(f[1].At) || f[0].By != f[1].By) {
			return false
		}
	}
	a.Completed, b.Completed, a.Cancelled, b.Cancelled = nil, nil, nil, nil
	return fmt.Sprint(a) == fmt.Sprint(b)
}

// orders R5: every Role creates Orders.
func TestTheOwnerAndAnAdminCreateOrders(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})

	for _, cookie := range []*http.Cookie{s.owner, s.admin} {
		if rec := h.createOrder(orderOf(lineIn{tea.ID, 1}), cookie); rec.Code != http.StatusCreated {
			t.Errorf("status = %d, want 201; body %s", rec.Code, rec.Body)
		}
	}
	if stock := h.stockOf(tea.ID, s.owner); stock != 8 {
		t.Errorf("stock = %d, want 8", stock)
	}
}

// orders R3: a new price leaves an Order already created as it was.
func TestAnOrderKeepsThePriceItWasCreatedWith(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 4500, Stock: 10})
	h.mustCreateOrder(s.staff, lineIn{tea.ID, 2})

	if rec := h.updateProduct(tea.ID, map[string]any{"price": 9900}, s.owner); rec.Code != http.StatusOK {
		t.Fatalf("update product: status = %d; body %s", rec.Code, rec.Body)
	}

	got := decode[orderBody](t, h.getOrder(1, s.staff))
	if got.Lines[0].Price != 4500 || got.Lines[0].Amount != 9000 || got.Total != 9000 {
		t.Errorf("order = %+v, want the price of 4500 it was created with", got)
	}
	if list := h.listOrders("", s.staff); list.Orders[0].Total != 9000 {
		t.Errorf("listed total = %d, want 9000", list.Orders[0].Total)
	}
}

// orders R2: numbers count up from 1 in the order Orders are created, each used once,
// even after a refused Order.
func TestOrderNumbersCountUpAndAreNeverReused(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})

	first := h.mustCreateOrder(s.staff, lineIn{tea.ID, 1})
	if rec := h.createOrder(orderOf(lineIn{tea.ID, 50}), s.staff); rec.Code != http.StatusConflict {
		t.Fatalf("an Order for 50 of 9: status = %d, want 409", rec.Code)
	}
	second := h.mustCreateOrder(s.staff, lineIn{tea.ID, 1})
	third := h.mustCreateOrder(s.staff, lineIn{tea.ID, 1})

	if first.Number != 1 || second.Number <= first.Number || third.Number <= second.Number {
		t.Errorf("numbers = %d, %d, %d; want them counting up from 1", first.Number, second.Number, third.Number)
	}
}

// orders R8: an Order with lines asking for more than their stock is refused whole, naming
// each such line's Product and stock. Nothing is taken.
func TestAnOrderAskingForTooMuchIsRefusedWholeNamingEachShortLine(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})
	cup := h.seed(store.Product{Name: "Cup", Sku: "CUP-01", Price: 100, Stock: 18})
	lid := h.seed(store.Product{Name: "Lid", Sku: "LID-01", Price: 100, Stock: 2})

	rec := h.createOrder(orderOf(lineIn{tea.ID, 10}, lineIn{cup.ID, 19}, lineIn{lid.ID, 3}), s.staff)

	assertProblem(t, rec, http.StatusConflict, "insufficient-stock")
	got := decode[insufficientStock](t, rec)
	if len(got.Products) != 2 || got.Products[0].ID != cup.ID || got.Products[0].Stock != 18 ||
		got.Products[1].ID != lid.ID || got.Products[1].Stock != 2 {
		t.Errorf("products = %+v, want Cup with 18 and Lid with 2", got.Products)
	}
	if !strings.Contains(got.Detail, "Only 18 in stock.") || !strings.Contains(got.Detail, "Only 2 in stock.") {
		t.Errorf("detail = %q, want each stock", got.Detail)
	}
	h.assertNothingTaken(s.owner, map[int64]int64{tea.ID: 10, cup.ID: 18, lid.ID: 2})
}

// orders R6, products R12: only Active Products are ordered.
func TestAnOrderNamingAnArchivedProductIsRefused(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})
	old := h.seed(store.Product{Name: "Old tea", Sku: "OLD-01", Price: 100, Stock: 10, Status: "archived"})

	rec := h.createOrder(orderOf(lineIn{tea.ID, 1}, lineIn{old.ID, 1}), s.staff)

	assertProblem(t, rec, http.StatusConflict, "product-archived")
	h.assertNothingTaken(s.owner, map[int64]int64{tea.ID: 10, old.ID: 10})
}

func TestAnOrderNamingAnUnknownProductIsNotFound(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})

	rec := h.createOrder(orderOf(lineIn{tea.ID, 1}, lineIn{404, 1}), s.staff)

	assertProblem(t, rec, http.StatusNotFound, "")
	h.assertNothingTaken(s.owner, map[int64]int64{tea.ID: 10})
}

// orders R1, R3: one or more lines, each a different Product with a whole quantity of at
// least 1; a customer of up to 120 characters and a note of up to 200.
func TestCreateOrderRefusesWhatNoOrderCanBe(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})
	cup := h.seed(store.Product{Name: "Cup", Sku: "CUP-01", Price: 100, Stock: 10})

	for location, body := range map[string]map[string]any{
		"body.lines":              {"lines": []lineIn{}},
		"body.lines[2].productId": {"lines": []lineIn{{tea.ID, 1}, {cup.ID, 1}, {tea.ID, 2}}},
		"body.lines[1].quantity":  {"lines": []lineIn{{tea.ID, 1}, {cup.ID, 0}}},
		"body.lines[0].quantity":  {"lines": []map[string]any{{"productId": tea.ID, "quantity": 1.5}}},
		"body.customer":           {"customer": strings.Repeat("ก", 121), "lines": []lineIn{{tea.ID, 1}}},
		"body.note":               {"note": strings.Repeat("ก", 201), "lines": []lineIn{{tea.ID, 1}}},
	} {
		rec := h.createOrder(body, s.staff)
		assertFieldError(t, rec, location)
	}
	assertFieldError(t, h.createOrder(map[string]any{"customer": "x"}, s.staff), "body.lines")
	noProduct := h.createOrder(map[string]any{"lines": []map[string]any{{"quantity": 1}}}, s.staff)
	if got := decode[problem](t, noProduct); noProduct.Code != http.StatusUnprocessableEntity || !hasFieldError(got, "body.lines[0]") {
		t.Errorf("a line with no Product: status = %d, errors %+v; want 422 at body.lines[0]", noProduct.Code, got.Errors)
	}
	h.assertNothingTaken(s.owner, map[int64]int64{tea.ID: 10, cup.ID: 10})

	edge := map[string]any{"customer": strings.Repeat("ก", 120), "note": strings.Repeat("ก", 200), "lines": []lineIn{{tea.ID, 1}}}
	if rec := h.createOrder(edge, s.staff); rec.Code != http.StatusCreated {
		t.Errorf("a customer of 120 and a note of 200 characters: status = %d, want 201; body %s", rec.Code, rec.Body)
	}
}

// A total past what the API can count is refused, not wrapped around.
func TestAnOrderWhoseTotalIsTooLargeIsRefused(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	var lines []lineIn
	for _, sku := range []string{"A", "B", "C", "D", "E"} {
		p := h.seed(store.Product{Name: "Gold " + sku, Sku: "GOLD-" + sku, Price: 999999999, Stock: math.MaxInt32})
		lines = append(lines, lineIn{p.ID, math.MaxInt32})
	}

	rec := h.createOrder(orderOf(lines...), s.staff)

	assertFieldError(t, rec, "body.lines")
}

// orders R11, context ADR 0007: any Role completes a Pending Order, which records who and
// when, and no stock changes.
func TestEveryRoleCompletesAnOrderWhichRecordsWhoAndWhen(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})
	for range 3 {
		h.mustCreateOrder(s.owner, lineIn{tea.ID, 1})
	}
	h.clock.Advance(time.Hour)

	for i, who := range []struct {
		cookie *http.Cookie
		user   orderUserBody
	}{{s.owner, orderUserBody{1, "Somchai"}}, {s.admin, orderUserBody{2, "Somchai"}}, {s.staff, orderUserBody{3, "Somchai"}}} {
		rec := h.completeOrder(int64(i+1), who.cookie)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
		}
		got := decode[orderBody](t, rec)
		if got.Status != "completed" || got.Completed == nil || got.Completed.By != who.user ||
			!got.Completed.At.Equal(h.clock.Now()) || got.Cancelled != nil || got.CreatedBy.ID != 1 {
			t.Errorf("order %d = %+v", i+1, got)
		}
		if again := decode[orderBody](t, h.getOrder(int64(i+1), s.staff)); !equalOrders(again, got) {
			t.Errorf("get = %+v, want %+v", again, got)
		}
	}
	if stock := h.stockOf(tea.ID, s.owner); stock != 7 {
		t.Errorf("stock = %d, want 7: completing changes no stock", stock)
	}
	if page := h.listMovements("", s.owner); len(page.Movements) != 3 {
		t.Errorf("movements = %d, want only the 3 Orders'", len(page.Movements))
	}
}

// orders R12: the Owner and Admins cancel a Pending Order; it records who and when, and
// each line's quantity goes back as one Cancellation movement, even to a Product
// Archived since.
func TestTheOwnerAndAnAdminCancelAnOrderAndItsStockGoesBack(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})
	cup := h.seed(store.Product{Name: "Cup", Sku: "CUP-01", Price: 100, Stock: 5})
	h.mustCreateOrder(s.staff, lineIn{tea.ID, 4}, lineIn{cup.ID, 5})
	h.mustCreateOrder(s.staff, lineIn{tea.ID, 1})
	if rec := h.updateProduct(cup.ID, map[string]any{"status": "archived"}, s.owner); rec.Code != http.StatusOK {
		t.Fatalf("archive: status = %d; body %s", rec.Code, rec.Body)
	}
	h.clock.Advance(time.Hour)

	rec := h.cancelOrder(1, s.admin)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	got := decode[orderBody](t, rec)
	if got.Status != "cancelled" || got.Cancelled == nil || got.Cancelled.By != (orderUserBody{2, "Somchai"}) ||
		!got.Cancelled.At.Equal(h.clock.Now()) || got.Completed != nil {
		t.Errorf("order = %+v", got)
	}
	if h.stockOf(tea.ID, s.owner) != 9 || h.stockOf(cup.ID, s.owner) != 5 {
		t.Errorf("stock = %d and %d, want 9 and 5", h.stockOf(tea.ID, s.owner), h.stockOf(cup.ID, s.owner))
	}
	page := h.listMovements("kind=cancellation", s.owner)
	if len(page.Movements) != 2 {
		t.Fatalf("cancellations = %+v, want 2", page.Movements)
	}
	for i, want := range []struct{ product, change, after int64 }{{cup.ID, 5, 5}, {tea.ID, 4, 9}} {
		m := page.Movements[i]
		if m.Product.ID != want.product || m.Change != want.change || m.StockAfter != want.after || m.User.ID != 2 ||
			m.Order == nil || m.Order.ID != got.ID || m.Order.Number != 1 {
			t.Errorf("cancellation %d = %+v, want %+v by the Admin, linked to Order 1", i, m, want)
		}
	}

	if rec := h.cancelOrder(2, s.owner); rec.Code != http.StatusOK || decode[orderBody](t, rec).Cancelled.By.ID != 1 {
		t.Errorf("the Owner cancels: status = %d; body %s", rec.Code, rec.Body)
	}
}

// orders R12: Staff cannot cancel, and the Order stays as it was.
func TestStaffCannotCancelAnOrder(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})
	h.mustCreateOrder(s.staff, lineIn{tea.ID, 4})

	rec := h.cancelOrder(1, s.staff)

	assertProblem(t, rec, http.StatusForbidden, "")
	if got := decode[orderBody](t, h.getOrder(1, s.staff)); got.Status != "pending" {
		t.Errorf("status = %q, want pending", got.Status)
	}
	if stock := h.stockOf(tea.ID, s.staff); stock != 6 {
		t.Errorf("stock = %d, want 6", stock)
	}
}

// orders R13: Completed and Cancelled are final.
func TestACompletedOrCancelledOrderIsFinal(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})
	h.mustCreateOrder(s.staff, lineIn{tea.ID, 1})
	h.mustCreateOrder(s.staff, lineIn{tea.ID, 1})
	h.completeOrder(1, s.staff)
	h.cancelOrder(2, s.owner)
	completed, cancelled := decode[orderBody](t, h.getOrder(1, s.staff)), decode[orderBody](t, h.getOrder(2, s.staff))

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"complete a Completed": h.completeOrder(1, s.staff),
		"cancel a Completed":   h.cancelOrder(1, s.owner),
		"complete a Cancelled": h.completeOrder(2, s.staff),
		"cancel a Cancelled":   h.cancelOrder(2, s.owner),
	} {
		t.Run(name, func(t *testing.T) { assertProblem(t, rec, http.StatusConflict, "order-not-pending") })
	}
	if got := decode[orderBody](t, h.getOrder(1, s.staff)); !equalOrders(got, completed) {
		t.Errorf("order 1 = %+v, want %+v", got, completed)
	}
	if got := decode[orderBody](t, h.getOrder(2, s.staff)); !equalOrders(got, cancelled) {
		t.Errorf("order 2 = %+v, want %+v", got, cancelled)
	}
	if stock := h.stockOf(tea.ID, s.staff); stock != 9 {
		t.Errorf("stock = %d, want 9", stock)
	}
}

// A Cancel whose stock would not fit is refused as a conflict, not a server error.
func TestACancelThatWouldTakeStockPastItsLimitIsRefused(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: math.MaxInt32})
	h.mustCreateOrder(s.staff, lineIn{tea.ID, 1})
	h.receive(map[string]any{"productId": tea.ID, "quantity": 1}, s.staff)

	rec := h.cancelOrder(1, s.owner)

	assertProblem(t, rec, http.StatusConflict, "")
	if got := decode[orderBody](t, h.getOrder(1, s.staff)); got.Status != "pending" {
		t.Errorf("status = %q, want pending", got.Status)
	}
}

func TestAnUnknownOrderIsNotFound(t *testing.T) {
	h := newHarness(t)
	s := h.shop()

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"get":      h.getOrder(404, s.staff),
		"complete": h.completeOrder(404, s.staff),
		"cancel":   h.cancelOrder(404, s.owner),
	} {
		t.Run(name, func(t *testing.T) { assertProblem(t, rec, http.StatusNotFound, "") })
	}
}

// orders R15, R18: every Role sees every Order, newest first, with its number, when it
// was created, its customer, its number of lines, its total and its status, a page at a
// time.
func TestEveryRoleListsEveryOrderNewestFirst(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 4500, Stock: 100})
	cup := h.seed(store.Product{Name: "Cup", Sku: "CUP-01", Price: 1250, Stock: 100})
	h.mustCreateOrder(s.owner, lineIn{tea.ID, 1})
	h.mustCreateOrder(s.admin, lineIn{tea.ID, 1})
	h.clock.Advance(time.Minute)
	if rec := h.createOrder(map[string]any{"customer": "Khun Dao", "lines": []lineIn{{tea.ID, 2}, {cup.ID, 1}}}, s.staff); rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	h.completeOrder(1, s.staff)

	for _, cookie := range []*http.Cookie{s.owner, s.admin, s.staff} {
		page := h.listOrders("limit=2", cookie)
		want := orderListItem{Number: 3, Customer: "Khun Dao", LineCount: 2, Total: 2*4500 + 1250, Status: "pending"}
		if page.Total != 3 || len(page.Orders) != 2 {
			t.Fatalf("page = %+v, want 2 of 3", page)
		}
		first := page.Orders[0]
		if first.ID == 0 || !first.CreatedAt.Equal(h.clock.Now()) {
			t.Errorf("first = %+v, want an id and created now", first)
		}
		first.ID, first.CreatedAt = 0, time.Time{}
		if first != want || page.Orders[1].Number != 2 {
			t.Errorf("orders = %+v, want %+v then Order 2", page.Orders, want)
		}
	}
	if got := h.orderNumbers("limit=2&offset=2", s.staff); !slices.Equal(got, []int64{1}) {
		t.Errorf("second page = %v, want [1]", got)
	}
	if got := h.listOrders("limit=2&offset=2", s.staff); got.Orders[0].Status != "completed" {
		t.Errorf("order 1 = %+v, want completed", got.Orders[0])
	}
	// orders R17, R18: the detail too, whoever created it.
	if rec := h.getOrder(1, s.staff); rec.Code != http.StatusOK || decode[orderBody](t, rec).CreatedBy.ID != 1 {
		t.Errorf("staff open the Owner's Order: status = %d; body %s", rec.Code, rec.Body)
	}
}

// orders R16: searched by number or customer, and filtered by status.
func TestTheOrdersListIsSearchedByNumberOrCustomerAndFilteredByStatus(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 100})
	for _, customer := range []string{"Khun Dao", "Somsri", "dao 50%", "Order 1 Ltd", ""} {
		body := map[string]any{"lines": []lineIn{{tea.ID, 1}}}
		if customer != "" {
			body["customer"] = customer
		}
		if rec := h.createOrder(body, s.staff); rec.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", rec.Code, rec.Body)
		}
	}
	h.completeOrder(1, s.staff)
	h.cancelOrder(2, s.owner)

	for query, want := range map[string][]int64{
		"search=DAO":                     {3, 1},
		"search=+dao+":                   {3, 1},
		"search=1":                       {4, 1},
		"search=%25":                     {3},
		"search=":                        {5, 4, 3, 2, 1},
		"status=pending":                 {5, 4, 3},
		"status=completed":               {1},
		"status=cancelled":               {2},
		"search=dao&status=pending":      {3},
		"search=nobody":                  {},
		"search=somsri&status=pending":   {},
		"search=somsri&status=cancelled": {2},
	} {
		if got := h.orderNumbers(query, s.staff); !slices.Equal(got, want) {
			t.Errorf("%s: numbers = %v, want %v", query, got, want)
		}
	}
	if page := h.listOrders("search=dao", s.staff); page.Total != 2 {
		t.Errorf("total = %d, want 2: the count follows the search", page.Total)
	}
	rec := h.do(http.MethodGet, "/orders?status=open", nil, s.staff)
	assertProblem(t, rec, http.StatusUnprocessableEntity, "")
}

// stock-movements R11: a movement made by an Order links to it, with its number.
func TestStockMovementsMadeByAnOrderLinkToIt(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 0})
	h.receive(map[string]any{"productId": tea.ID, "quantity": 5}, s.staff)
	h.mustCreateOrder(s.staff, lineIn{tea.ID, 1})
	second := h.mustCreateOrder(s.staff, lineIn{tea.ID, 2})

	page := h.listMovements("", s.staff)

	if len(page.Movements) != 3 {
		t.Fatalf("movements = %+v, want 3", page.Movements)
	}
	if m := page.Movements[0]; m.Kind != "order" || m.Order == nil || m.Order.ID != second.ID || m.Order.Number != 2 || m.Change != -2 {
		t.Errorf("newest = %+v, want Order 2's movement linked to it", m)
	}
	if m := page.Movements[2]; m.Kind != "receive" || m.Order != nil {
		t.Errorf("oldest = %+v, want the Receive with no Order", m)
	}
}

// users-and-access R4: every Orders operation needs a live session.
func TestEveryOrderOperationNeedsASession(t *testing.T) {
	h := newHarness(t)
	h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 100, Stock: 10})

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"list":     h.do(http.MethodGet, "/orders", nil),
		"get":      h.getOrder(1),
		"create":   h.createOrder(orderOf(lineIn{tea.ID, 1})),
		"complete": h.completeOrder(1),
		"cancel":   h.cancelOrder(1),
	} {
		t.Run(name, func(t *testing.T) { assertProblem(t, rec, http.StatusUnauthorized, "") })
	}
	if stock := h.stockOf(tea.ID, h.session("staff@example.com")); stock != 10 {
		t.Errorf("stock = %d, want 10", stock)
	}
}
