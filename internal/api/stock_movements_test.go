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

type movementBody struct {
	ID      int64     `json:"id"`
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`
	Product struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		SKU  string `json:"sku"`
	} `json:"product"`
	Change     int64 `json:"change"`
	StockAfter int64 `json:"stockAfter"`
	User       struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"user"`
	Note  string `json:"note"`
	Order *struct {
		ID     int64 `json:"id"`
		Number int64 `json:"number"`
	} `json:"order"`
}

type movementPage struct {
	Movements  []movementBody `json:"movements"`
	NextBefore *int64         `json:"nextBefore"`
}

type insufficientStock struct {
	problem
	Products []struct {
		ID    int64 `json:"id"`
		Stock int64 `json:"stock"`
	} `json:"products"`
}

func (h *harness) receive(body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, "/stock-movements/receive", body, cookies...)
}

func (h *harness) adjust(body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, "/stock-movements/adjust", body, cookies...)
}

func (h *harness) listMovements(query string, cookies ...*http.Cookie) movementPage {
	h.t.Helper()
	rec := h.do(http.MethodGet, "/stock-movements?"+query, nil, cookies...)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("list stock movements %q: status = %d, want 200; body %s", query, rec.Code, rec.Body)
	}
	return decode[movementPage](h.t, rec)
}

// stockOf is the Product's stock as the API shows it.
func (h *harness) stockOf(id int64, cookie *http.Cookie) int64 {
	h.t.Helper()
	return decode[productBody](h.t, h.getProduct(id, cookie)).Stock
}

// assertFieldError asserts a 422 that blames the field at location. A field left out is
// blamed on the object that should hold it, naming the field.
func assertFieldError(t *testing.T, rec *httptest.ResponseRecorder, location string) {
	t.Helper()
	assertProblem(t, rec, http.StatusUnprocessableEntity, "")
	got := decode[problem](t, rec)
	parent, field, _ := strings.Cut(location, ".")
	for _, e := range got.Errors {
		if e.Location == location || e.Location == parent && strings.Contains(e.Message, "property "+field+" ") {
			return
		}
	}
	t.Errorf("errors = %+v, want one at %s", got.Errors, location)
}

// stock-movements R2, R4, R11: every Role may Receive; the movement records its kind,
// Product, change, stock after, time, User and note, and the stock changes with it.
func TestStaffReceiveStockAndTheMovementRecordsIt(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Stock: 5})

	rec := h.receive(map[string]any{"productId": tea.ID, "quantity": 12, "note": "Delivery 12"}, s.staff)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body)
	}
	got := decode[movementBody](t, rec)
	if got.ID == 0 || got.Kind != "receive" || got.Product.ID != tea.ID || got.Product.Name != "Thai tea" || got.Product.SKU != "TEA-01" ||
		got.Change != 12 || got.StockAfter != 17 || got.User.ID != 3 || got.User.Name != "Somchai" ||
		got.Note != "Delivery 12" || !got.At.Equal(h.clock.Now()) || got.Order != nil {
		t.Errorf("movement = %+v", got)
	}
	if !strings.Contains(rec.Body.String(), `"order":null`) {
		t.Errorf("body %s, want order null", rec.Body)
	}
	if stock := h.stockOf(tea.ID, s.staff); stock != 17 {
		t.Errorf("stock = %d, want 17", stock)
	}
	if page := h.listMovements("", s.owner); len(page.Movements) != 1 || page.Movements[0] != got {
		t.Errorf("listed = %+v, want [%+v]", page.Movements, got)
	}
}

// stock-movements R4: the Owner and Admins receive too.
func TestTheOwnerAndAnAdminReceiveStock(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01"})

	for _, cookie := range []*http.Cookie{s.owner, s.admin} {
		if rec := h.receive(map[string]any{"productId": tea.ID, "quantity": 1}, cookie); rec.Code != http.StatusCreated {
			t.Errorf("status = %d, want 201; body %s", rec.Code, rec.Body)
		}
	}
	if stock := h.stockOf(tea.ID, s.owner); stock != 2 {
		t.Errorf("stock = %d, want 2", stock)
	}
}

// stock-movements R4: a whole number of units, at least 1.
func TestReceiveNeedsAWholeQuantityOfAtLeastOne(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Stock: 5})

	for _, quantity := range []any{0, -3, 1.5, nil} {
		body := map[string]any{"productId": tea.ID}
		if quantity != nil {
			body["quantity"] = quantity
		}
		rec := h.receive(body, s.staff)
		assertFieldError(t, rec, "body.quantity")
	}
	if stock := h.stockOf(tea.ID, s.staff); stock != 5 {
		t.Errorf("stock = %d, want 5", stock)
	}
}

// stock-movements R2: the note is optional, up to 200 characters.
func TestAReceiveNoteIsOptionalAndUpTo200Characters(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01"})

	none := h.receive(map[string]any{"productId": tea.ID, "quantity": 1}, s.staff)
	long := h.receive(map[string]any{"productId": tea.ID, "quantity": 1, "note": strings.Repeat("ก", 201)}, s.staff)
	most := h.receive(map[string]any{"productId": tea.ID, "quantity": 1, "note": strings.Repeat("ก", 200)}, s.staff)

	if none.Code != http.StatusCreated || decode[movementBody](t, none).Note != "" {
		t.Errorf("no note: status = %d, body %s, want 201 with an empty note", none.Code, none.Body)
	}
	assertFieldError(t, long, "body.note")
	if most.Code != http.StatusCreated {
		t.Errorf("200 characters: status = %d, want 201; body %s", most.Code, most.Body)
	}
}

// stock-movements R5: the Owner and Admins adjust up or down.
func TestTheOwnerAndAnAdminAdjustStockUpOrDown(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Stock: 5})

	down := h.adjust(map[string]any{"productId": tea.ID, "change": -2, "note": "Two broken"}, s.owner)
	up := h.adjust(map[string]any{"productId": tea.ID, "change": 1, "note": "Found one"}, s.admin)

	if down.Code != http.StatusCreated || up.Code != http.StatusCreated {
		t.Fatalf("status = %d and %d, want 201; bodies %s %s", down.Code, up.Code, down.Body, up.Body)
	}
	d, u := decode[movementBody](t, down), decode[movementBody](t, up)
	if d.Kind != "adjustment" || d.Change != -2 || d.StockAfter != 3 || d.Note != "Two broken" || d.User.ID != 1 {
		t.Errorf("down = %+v", d)
	}
	if u.Kind != "adjustment" || u.Change != 1 || u.StockAfter != 4 || u.User.ID != 2 {
		t.Errorf("up = %+v", u)
	}
	if stock := h.stockOf(tea.ID, s.staff); stock != 4 {
		t.Errorf("stock = %d, want 4", stock)
	}
}

// stock-movements R5: Staff make no Adjustments, whatever they send.
func TestStaffCannotAdjustStock(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Stock: 5})

	for _, body := range []any{
		map[string]any{"productId": tea.ID, "change": 3, "note": "count"},
		map[string]any{"productId": tea.ID, "change": 0},
	} {
		assertProblem(t, h.adjust(body, s.staff), http.StatusForbidden, "")
	}
	if stock := h.stockOf(tea.ID, s.staff); stock != 5 {
		t.Errorf("stock = %d, want 5", stock)
	}
	if page := h.listMovements("", s.owner); len(page.Movements) != 0 {
		t.Errorf("movements = %+v, want none", page.Movements)
	}
}

// stock-movements R5: a whole number other than 0.
func TestAnAdjustmentChangesByAWholeNumberOtherThanZero(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Stock: 5})

	for _, change := range []any{0, 1.5, nil} {
		body := map[string]any{"productId": tea.ID, "note": "count"}
		if change != nil {
			body["change"] = change
		}
		assertFieldError(t, h.adjust(body, s.owner), "body.change")
	}
	if stock := h.stockOf(tea.ID, s.owner); stock != 5 {
		t.Errorf("stock = %d, want 5", stock)
	}
}

// stock-movements R5: an Adjustment needs a note saying why.
func TestAnAdjustmentNeedsANote(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Stock: 5})

	for _, note := range []any{nil, "", "   ", strings.Repeat("a", 201)} {
		body := map[string]any{"productId": tea.ID, "change": 1}
		if note != nil {
			body["note"] = note
		}
		assertFieldError(t, h.adjust(body, s.owner), "body.note")
	}
	if stock := h.stockOf(tea.ID, s.owner); stock != 5 {
		t.Errorf("stock = %d, want 5", stock)
	}
}

// stock-movements R6: refused, saying how many are in stock.
func TestAnAdjustmentBelowZeroIsRefusedWithTheStock(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Stock: 18})

	rec := h.adjust(map[string]any{"productId": tea.ID, "change": -19, "note": "count"}, s.owner)

	assertProblem(t, rec, http.StatusConflict, "insufficient-stock")
	got := decode[insufficientStock](t, rec)
	if len(got.Products) != 1 || got.Products[0].ID != tea.ID || got.Products[0].Stock != 18 {
		t.Errorf("products = %+v, want Thai tea with 18", got.Products)
	}
	if !strings.Contains(got.Detail, "18") {
		t.Errorf("detail = %q, want it to say 18", got.Detail)
	}
	if stock := h.stockOf(tea.ID, s.owner); stock != 18 {
		t.Errorf("stock = %d, want 18", stock)
	}
	if ok := h.adjust(map[string]any{"productId": tea.ID, "change": -18, "note": "count"}, s.owner); ok.Code != http.StatusCreated {
		t.Errorf("down to 0: status = %d, want 201; body %s", ok.Code, ok.Body)
	}
}

// stock-movements R9, products R12.
func TestAnArchivedProductCannotBeReceivedOrAdjusted(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	old := h.seed(store.Product{Name: "Old tea", Sku: "OLD-01", Stock: 5, Status: "archived"})

	assertProblem(t, h.receive(map[string]any{"productId": old.ID, "quantity": 1}, s.staff), http.StatusConflict, "product-archived")
	assertProblem(t, h.adjust(map[string]any{"productId": old.ID, "change": -1, "note": "count"}, s.owner), http.StatusConflict, "product-archived")
	// Refused as Archived, not as short: it is not offered at all.
	assertProblem(t, h.adjust(map[string]any{"productId": old.ID, "change": -9, "note": "count"}, s.owner), http.StatusConflict, "product-archived")

	if stock := h.stockOf(old.ID, s.owner); stock != 5 {
		t.Errorf("stock = %d, want 5", stock)
	}
}

func TestReceiveAndAdjustAnswer404ForNoSuchProduct(t *testing.T) {
	h := newHarness(t)
	s := h.shop()

	assertProblem(t, h.receive(map[string]any{"productId": 404, "quantity": 1}, s.staff), http.StatusNotFound, "")
	assertProblem(t, h.adjust(map[string]any{"productId": 404, "change": 1, "note": "count"}, s.owner), http.StatusNotFound, "")
}

// A stock column holds no more than 2147483647; past it, the request is at fault.
func TestAReceivePastTheLargestStockIsRefused(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Stock: math.MaxInt32 - 1})

	assertFieldError(t, h.receive(map[string]any{"productId": tea.ID, "quantity": 2}, s.staff), "body.quantity")
	assertFieldError(t, h.adjust(map[string]any{"productId": tea.ID, "change": 2, "note": "count"}, s.owner), "body.change")
}

// users-and-access R4: no operation without a session.
func TestStockMovementsNeedASession(t *testing.T) {
	h := newHarness(t)
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Stock: 5})

	assertProblem(t, h.do(http.MethodGet, "/stock-movements", nil), http.StatusUnauthorized, "")
	assertProblem(t, h.receive(map[string]any{"productId": tea.ID, "quantity": 1}), http.StatusUnauthorized, "")
	assertProblem(t, h.adjust(map[string]any{"productId": tea.ID, "change": 1, "note": "x"}), http.StatusUnauthorized, "")
}

// stock-movements R10, R12: newest first, a page at a time, and every Role sees every
// movement.
func TestEveryRoleListsEveryMovementNewestFirstAPageAtATime(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Stock: 5})
	var made []int64
	for i := range 5 {
		rec := h.adjust(map[string]any{"productId": tea.ID, "change": i + 1, "note": "count"}, s.owner)
		made = append(made, decode[movementBody](t, rec).ID)
	}

	var got []int64
	query := "limit=2"
	for range 3 {
		page := h.listMovements(query, s.staff)
		for _, m := range page.Movements {
			got = append(got, m.ID)
		}
		if page.NextBefore == nil {
			break
		}
		query = fmt.Sprintf("limit=2&before=%d", *page.NextBefore)
	}

	slices.Reverse(made)
	if !slices.Equal(got, made) {
		t.Errorf("ids = %v, want %v", got, made)
	}
	if last := h.listMovements("limit=2&before="+fmt.Sprint(made[3]), s.staff); last.NextBefore != nil {
		t.Errorf("last page nextBefore = %v, want null", *last.NextBefore)
	}
	if empty := h.do(http.MethodGet, "/stock-movements?kind=order", nil, s.staff); !strings.Contains(empty.Body.String(), `"movements":[]`) {
		t.Errorf("no movements: body %s, want an empty array", empty.Body)
	}
}

// stock-movements R10: by Product and by kind.
func TestListStockMovementsFiltersByProductAndKind(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Stock: 5})
	cup := h.seed(store.Product{Name: "Cup", Sku: "CUP-01", Stock: 5})
	h.receive(map[string]any{"productId": tea.ID, "quantity": 1}, s.staff)
	h.adjust(map[string]any{"productId": tea.ID, "change": -1, "note": "count"}, s.owner)
	h.receive(map[string]any{"productId": cup.ID, "quantity": 1}, s.staff)

	describe := func(page movementPage) []string {
		got := []string{}
		for _, m := range page.Movements {
			got = append(got, m.Product.SKU+" "+m.Kind)
		}
		return got
	}

	if got, want := describe(h.listMovements(fmt.Sprintf("productId=%d", tea.ID), s.staff)), []string{"TEA-01 adjustment", "TEA-01 receive"}; !slices.Equal(got, want) {
		t.Errorf("by Product = %v, want %v", got, want)
	}
	if got, want := describe(h.listMovements("kind=receive", s.staff)), []string{"CUP-01 receive", "TEA-01 receive"}; !slices.Equal(got, want) {
		t.Errorf("by kind = %v, want %v", got, want)
	}
	if got := describe(h.listMovements(fmt.Sprintf("productId=%d&kind=adjustment", cup.ID), s.staff)); len(got) != 0 {
		t.Errorf("both = %v, want none", got)
	}
	assertProblem(t, h.do(http.MethodGet, "/stock-movements?kind=stocktake", nil, s.staff), http.StatusUnprocessableEntity, "")
}

// dashboard-and-reports R1, stock-movements R10: the Shop's days run on Bangkok time, so a
// day starts at 17:00 UTC the day before.
func TestListStockMovementsFiltersByTheShopsDaysInBangkok(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01"})
	h.clock.now = time.Date(2026, 11, 6, 16, 59, 59, 0, time.UTC) // 23:59:59 on 6 November in Bangkok
	late := decode[movementBody](t, h.receive(map[string]any{"productId": tea.ID, "quantity": 1}, s.staff)).ID
	h.clock.now = time.Date(2026, 11, 6, 17, 0, 0, 0, time.UTC) // midnight starting 7 November
	early := decode[movementBody](t, h.receive(map[string]any{"productId": tea.ID, "quantity": 1}, s.staff)).ID
	h.clock.now = time.Date(2026, 11, 7, 16, 59, 59, 0, time.UTC) // the last second of 7 November
	last := decode[movementBody](t, h.receive(map[string]any{"productId": tea.ID, "quantity": 1}, s.staff)).ID

	ids := func(query string) []int64 {
		got := []int64{}
		for _, m := range h.listMovements(query, s.staff).Movements {
			got = append(got, m.ID)
		}
		return got
	}

	for query, want := range map[string][]int64{
		"from=2026-11-07&to=2026-11-07": {last, early},
		"from=2026-11-06&to=2026-11-06": {late},
		"from=2026-11-07":               {last, early},
		"to=2026-11-06":                 {late},
		"from=2026-11-08":               {},
		"from=2026-11-06&to=2026-11-07": {last, early, late},
	} {
		if got := ids(query); !slices.Equal(got, want) {
			t.Errorf("%s: ids = %v, want %v", query, got, want)
		}
	}
}

func TestListStockMovementsRefusesADateRangeThatEndsBeforeItStarts(t *testing.T) {
	h := newHarness(t)
	s := h.shop()

	assertFieldError(t, h.do(http.MethodGet, "/stock-movements?from=2026-11-08&to=2026-11-07", nil, s.staff), "query.to")
	assertFieldError(t, h.do(http.MethodGet, "/stock-movements?from=7-11-2026", nil, s.staff), "query.from")
}
