package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

type productBody struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	SKU          string `json:"sku"`
	Unit         string `json:"unit"`
	Price        int64  `json:"price"`
	MinimumStock int64  `json:"minimumStock"`
	Stock        int64  `json:"stock"`
	StockState   string `json:"stockState"`
	Status       string `json:"status"`
}

type productPage struct {
	Products []productBody `json:"products"`
	Total    int64         `json:"total"`
}

// shop has one User of each Role logged in, for the operations whose answer depends on it.
type shop struct {
	owner, admin, staff *http.Cookie
}

func (h *harness) shop() shop {
	h.t.Helper()
	h.addUser("owner@example.com", func(u *store.User) { u.Role = "owner" })
	h.addUser("admin@example.com", func(u *store.User) { u.Role = "admin" })
	h.addUser("staff@example.com")
	return shop{h.session("owner@example.com"), h.session("admin@example.com"), h.session("staff@example.com")}
}

func (h *harness) createProduct(body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, "/products", body, cookies...)
}

func (h *harness) updateProduct(id int64, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPatch, fmt.Sprintf("/products/%d", id), body, cookies...)
}

func (h *harness) getProduct(id int64, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodGet, fmt.Sprintf("/products/%d", id), nil, cookies...)
}

func (h *harness) listProducts(query string, cookies ...*http.Cookie) productPage {
	h.t.Helper()
	rec := h.do(http.MethodGet, "/products?"+query, nil, cookies...)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("list products %q: status = %d, want 200; body %s", query, rec.Code, rec.Body)
	}
	return decode[productPage](h.t, rec)
}

// seed adds a Product with a stock, which no operation can set (context ADR 0006).
func (h *harness) seed(p store.Product) store.Product {
	h.t.Helper()
	if p.Unit == "" {
		p.Unit = "pcs"
	}
	if p.Status == "" {
		p.Status = "active"
	}
	if p.Sku == "" {
		p.Sku = fmt.Sprintf("SKU-%d", len(p.Name)+int(p.Stock))
	}
	return h.store.AddProduct(p)
}

func productNames(page productPage) []string {
	got := []string{}
	for _, p := range page.Products {
		got = append(got, p.Name)
	}
	return got
}

var validProduct = map[string]any{"name": "Thai tea", "sku": "TEA-01", "unit": "box", "price": 12050, "minimumStock": 5}

// with copies validProduct and sets one field.
func with(field string, value any) map[string]any {
	body := map[string]any{}
	for k, v := range validProduct {
		body[k] = v
	}
	if value == nil {
		delete(body, field)
	} else {
		body[field] = value
	}
	return body
}

// products R1, R5-R8, R11: a new Product is Active with stock 0; the price is whole satang.
func TestCreateProductStartsActiveWithStockZero(t *testing.T) {
	h := newHarness(t)
	s := h.shop()

	rec := h.createProduct(validProduct, s.owner)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body)
	}
	got := decode[productBody](t, rec)
	want := productBody{
		ID: got.ID, Name: "Thai tea", SKU: "TEA-01", Unit: "box", Price: 12050, MinimumStock: 5,
		Stock: 0, StockState: "out-of-stock", Status: "active",
	}
	if got != want || got.ID == 0 {
		t.Errorf("product = %+v, want %+v", got, want)
	}
	if again := decode[productBody](t, h.getProduct(got.ID, s.staff)); again != got {
		t.Errorf("got %+v after, want %+v", again, got)
	}
}

// users-and-access R11: the Owner and Admins add Products.
func TestAnAdminCreatesAProduct(t *testing.T) {
	h := newHarness(t)
	s := h.shop()

	rec := h.createProduct(validProduct, s.admin)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body)
	}
}

// products R8.
func TestCreateProductMinimumStockIsZeroByDefault(t *testing.T) {
	h := newHarness(t)
	s := h.shop()

	rec := h.createProduct(with("minimumStock", nil), s.owner)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body)
	}
	if got := decode[productBody](t, rec); got.MinimumStock != 0 {
		t.Errorf("minimum stock = %d, want 0", got.MinimumStock)
	}
}

// products R2-R5, R8 at their edges.
func TestCreateProductAcceptsTheEdgesOfEachRule(t *testing.T) {
	h := newHarness(t)
	s := h.shop()

	for name, body := range map[string]map[string]any{
		"a name of 120 characters in any language": with("name", strings.Repeat("ก", 120)),
		"a SKU of 40 letters, digits and hyphens":  with("sku", strings.Repeat("aZ9-", 10)),
		"a unit of 20 characters":                  with("unit", strings.Repeat("u", 20)),
		"a unit with a space":                      with("unit", "6 pack"),
		"a price of 0":                             with("price", 0),
		"the highest price, ฿9,999,999.99":         with("price", 999999999),
		"a minimum of 0":                           with("minimumStock", 0),
		"a large minimum":                          with("minimumStock", 1000000),
	} {
		// Each needs a SKU of its own, or it would meet the one before it.
		if _, ok := body["sku"]; ok && body["sku"] == validProduct["sku"] {
			body["sku"] = fmt.Sprintf("OK-%d", len(name))
		}
		if rec := h.createProduct(body, s.owner); rec.Code != http.StatusCreated {
			t.Errorf("%s: status = %d, want 201; body %s", name, rec.Code, rec.Body)
		}
	}
}

func TestCreateProductRefusesWhatTheRulesRefuse(t *testing.T) {
	h := newHarness(t)
	s := h.shop()

	for name, body := range map[string]map[string]any{
		"no name":                           with("name", nil),
		"an empty name":                     with("name", ""),
		"a name of only spaces":             with("name", "   "),
		"a name of only no-break spaces":    with("name", "\u00a0\u00a0"),
		"a name of only ideographic spaces": with("name", "\u3000"),
		"a name of only line breaks":        with("name", " \u0085\u2028"),
		"a name of 121 characters":          with("name", strings.Repeat("ก", 121)),
		"no SKU":                            with("sku", nil),
		"an empty SKU":                      with("sku", ""),
		"a SKU with a space":                with("sku", "TEA 01"),
		"a SKU with an underscore":          with("sku", "TEA_01"),
		"a SKU with a Thai letter":          with("sku", "ชา-01"),
		"a SKU of 41 characters":            with("sku", strings.Repeat("A", 41)),
		"no unit":                           with("unit", nil),
		"an empty unit":                     with("unit", ""),
		"a unit of only spaces":             with("unit", " "),
		"a unit of only no-break spaces":    with("unit", "\u00a0"),
		"a unit of only ideographic spaces": with("unit", "\u3000"),
		"a unit of 21 characters":           with("unit", strings.Repeat("u", 21)),
		"no price":                          with("price", nil),
		"a negative price":                  with("price", -1),
		"a price over ฿9,999,999.99":        with("price", 1000000000),
		"a price with a fraction":           with("price", 12.5),
		"a price as text":                   with("price", "120.50"),
		"a negative minimum":                with("minimumStock", -1),
		"a minimum with a fraction":         with("minimumStock", 1.5),
		"a stock":                           with("stock", 10),
		"a status":                          with("status", "archived"),
		"a body that is not an object":      nil,
	} {
		rec := h.createProduct(body, s.owner)
		if name == "a body that is not an object" {
			rec = h.do(http.MethodPost, "/products", []int{1}, s.owner)
		}
		assertProblem(t, rec, http.StatusUnprocessableEntity, "")
		if t.Failed() {
			t.Fatalf("%s: see above", name)
		}
	}
	if page := h.listProducts("", s.owner); len(page.Products) != 0 {
		t.Errorf("products = %+v, want none: nothing refused may be kept", page.Products)
	}
}

// products R3.
func TestCreateProductRefusesASKUAnotherProductHasInAnyLetterCase(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	h.createProduct(with("sku", "Tea-01"), s.owner)

	for _, sku := range []string{"Tea-01", "TEA-01", "tea-01", "tEA-01"} {
		rec := h.createProduct(with("sku", sku), s.admin)

		assertProblem(t, rec, http.StatusConflict, "sku-taken")
	}
	if page := h.listProducts("", s.owner); len(page.Products) != 1 {
		t.Errorf("products = %+v, want only the first", page.Products)
	}
}

// products R3: an Archived Product keeps its SKU.
func TestCreateProductRefusesTheSKUOfAnArchivedProduct(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	h.seed(store.Product{Name: "Old tea", Sku: "TEA-01", Status: "archived"})

	rec := h.createProduct(validProduct, s.owner)

	assertProblem(t, rec, http.StatusConflict, "sku-taken")
}

// users-and-access R11, R23: the API refuses whatever the web app shows.
func TestStaffCannotCreateOrUpdateAProduct(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01"})

	// A body with every mistake, to show the refusal does not depend on the body.
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"create":         h.createProduct(validProduct, s.staff),
		"create invalid": h.createProduct(map[string]any{"name": ""}, s.staff),
		"update":         h.updateProduct(tea.ID, map[string]any{"name": "Changed"}, s.staff),
		"update invalid": h.updateProduct(tea.ID, map[string]any{"price": -1}, s.staff),
		"update unknown": h.updateProduct(tea.ID+1, map[string]any{"name": "Changed"}, s.staff),
	} {
		assertProblem(t, rec, http.StatusForbidden, "")
		if t.Failed() {
			t.Fatalf("%s: see above", name)
		}
	}
	page := h.listProducts("", s.staff)
	if len(page.Products) != 1 || page.Products[0].Name != "Thai tea" {
		t.Errorf("products = %+v, want the one Product unchanged", page.Products)
	}
}

// users-and-access R4, R26: no session, no Products.
func TestEveryProductOperationAnswers401WithoutALiveSession(t *testing.T) {
	h := newHarness(t)
	h.shop()
	tea := h.seed(store.Product{Name: "Thai tea"})

	for name, call := range map[string]func(...*http.Cookie) *httptest.ResponseRecorder{
		"list": func(c ...*http.Cookie) *httptest.ResponseRecorder {
			return h.do(http.MethodGet, "/products", nil, c...)
		},
		"get":    func(c ...*http.Cookie) *httptest.ResponseRecorder { return h.getProduct(tea.ID, c...) },
		"create": func(c ...*http.Cookie) *httptest.ResponseRecorder { return h.createProduct(validProduct, c...) },
		"update": func(c ...*http.Cookie) *httptest.ResponseRecorder {
			return h.updateProduct(tea.ID, map[string]any{}, c...)
		},
	} {
		assertProblem(t, call(), http.StatusUnauthorized, "")
		assertProblem(t, call(&http.Cookie{Name: "session", Value: "not-a-session"}), http.StatusUnauthorized, "session-ended")
		if t.Failed() {
			t.Fatalf("%s: see above", name)
		}
	}
}

// products R15: every Role sees the same Product.
func TestEveryRoleGetsAProduct(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Unit: "box", Price: 12050, Stock: 3, MinimumStock: 5})
	want := productBody{
		ID: tea.ID, Name: "Thai tea", SKU: "TEA-01", Unit: "box", Price: 12050, MinimumStock: 5, Stock: 3,
		StockState: "low", Status: "active",
	}

	for role, cookie := range map[string]*http.Cookie{"owner": s.owner, "admin": s.admin, "staff": s.staff} {
		rec := h.getProduct(tea.ID, cookie)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200; body %s", role, rec.Code, rec.Body)
		}
		if got := decode[productBody](t, rec); got != want {
			t.Errorf("%s: product = %+v, want %+v", role, got, want)
		}
	}
}

func TestGetProductAnswers404ForNoSuchProduct(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea"})

	assertProblem(t, h.getProduct(tea.ID+1, s.staff), http.StatusNotFound, "")
	assertProblem(t, h.do(http.MethodGet, "/products/abc", nil, s.staff), http.StatusUnprocessableEntity, "")
}

// products R3, R5, R8, R11: any of the fields changes, and the others stay.
func TestUpdateProductChangesTheFieldsItIsGiven(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Unit: "box", Price: 12050, MinimumStock: 5, Stock: 3})
	base := productBody{
		ID: tea.ID, Name: "Thai tea", SKU: "TEA-01", Unit: "box", Price: 12050, MinimumStock: 5, Stock: 3,
		StockState: "low", Status: "active",
	}

	for name, tc := range map[string]struct {
		body   map[string]any
		change func(*productBody)
	}{
		"name":            {map[string]any{"name": "ชาไทย"}, func(p *productBody) { p.Name = "ชาไทย" }},
		"SKU":             {map[string]any{"sku": "TEA-02"}, func(p *productBody) { p.SKU = "TEA-02" }},
		"SKU letter case": {map[string]any{"sku": "tea-01"}, func(p *productBody) { p.SKU = "tea-01" }},
		"unit":            {map[string]any{"unit": "pack"}, func(p *productBody) { p.Unit = "pack" }},
		"price":           {map[string]any{"price": 0}, func(p *productBody) { p.Price = 0 }},
		"minimum stock":   {map[string]any{"minimumStock": 0}, func(p *productBody) { p.MinimumStock = 0; p.StockState = "in-stock" }},
		"status":          {map[string]any{"status": "archived"}, func(p *productBody) { p.Status = "archived" }},
		"nothing":         {map[string]any{}, func(*productBody) {}},
	} {
		h.updateProduct(tea.ID, map[string]any{"name": "Thai tea", "sku": "TEA-01", "unit": "box", "price": 12050, "minimumStock": 5, "status": "active"}, s.owner)

		rec := h.updateProduct(tea.ID, tc.body, s.admin)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200; body %s", name, rec.Code, rec.Body)
		}
		want := base
		tc.change(&want)
		if got := decode[productBody](t, rec); got != want {
			t.Errorf("%s: product = %+v, want %+v", name, got, want)
		}
		if got := decode[productBody](t, h.getProduct(tea.ID, s.staff)); got != want {
			t.Errorf("%s: got %+v after, want %+v", name, got, want)
		}
	}
}

// products R6, context ADR 0006: no operation sets stock.
func TestUpdateProductRefusesAStockAndLeavesTheStockAlone(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Stock: 7, MinimumStock: 2})

	assertProblem(t, h.updateProduct(tea.ID, map[string]any{"stock": 0}, s.owner), http.StatusUnprocessableEntity, "")
	assertProblem(t, h.updateProduct(tea.ID, map[string]any{"stockState": "low"}, s.owner), http.StatusUnprocessableEntity, "")
	rec := h.updateProduct(tea.ID, map[string]any{"name": "Green tea", "price": 100, "minimumStock": 9, "status": "archived"}, s.owner)

	if got := decode[productBody](t, rec); got.Stock != 7 || got.StockState != "low" {
		t.Errorf("stock = %d, state = %s, want 7 and low", got.Stock, got.StockState)
	}
}

func TestUpdateProductRefusesWhatTheRulesRefuse(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01"})

	for name, body := range map[string]map[string]any{
		"an empty name":                     {"name": ""},
		"a name of only spaces":             {"name": "  "},
		"a name of only no-break spaces":    {"name": "\u00a0"},
		"a name of only ideographic spaces": {"name": "\u3000"},
		"a unit of only spaces":             {"unit": " "},
		"a unit of only no-break spaces":    {"unit": "\u00a0"},
		"a unit of only ideographic spaces": {"unit": "\u3000"},
		"a name of 121":                     {"name": strings.Repeat("ก", 121)},
		"an empty SKU":                      {"sku": ""},
		"a SKU with a space":                {"sku": "TEA 01"},
		"a SKU of 41":                       {"sku": strings.Repeat("A", 41)},
		"an empty unit":                     {"unit": ""},
		"a unit of 21":                      {"unit": strings.Repeat("u", 21)},
		"a negative price":                  {"price": -1},
		"a price over the limit":            {"price": 1000000000},
		"a fractional price":                {"price": 0.5},
		"a negative minimum":                {"minimumStock": -1},
		"an unknown status":                 {"status": "deleted"},
	} {
		assertProblem(t, h.updateProduct(tea.ID, body, s.owner), http.StatusUnprocessableEntity, "")
		if t.Failed() {
			t.Fatalf("%s: see above", name)
		}
	}
}

func TestUpdateProductAnswers404ForNoSuchProduct(t *testing.T) {
	h := newHarness(t)
	s := h.shop()

	rec := h.updateProduct(99, map[string]any{"name": "Thai tea"}, s.owner)

	assertProblem(t, rec, http.StatusNotFound, "")
}

// products R3: the same refusal as Create, and a Product keeps its own SKU.
func TestUpdateProductRefusesASKUAnotherProductHasInAnyLetterCase(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	h.seed(store.Product{Name: "Thai tea", Sku: "Tea-01"})
	green := h.seed(store.Product{Name: "Green tea", Sku: "TEA-02"})
	h.seed(store.Product{Name: "Old tea", Sku: "OLD", Status: "archived"})

	for _, sku := range []string{"Tea-01", "TEA-01", "tea-01"} {
		assertProblem(t, h.updateProduct(green.ID, map[string]any{"sku": sku}, s.owner), http.StatusConflict, "sku-taken")
	}
	assertProblem(t, h.updateProduct(green.ID, map[string]any{"sku": "old"}, s.owner), http.StatusConflict, "sku-taken")
	if rec := h.updateProduct(green.ID, map[string]any{"sku": "tea-02", "name": "Green tea"}, s.owner); rec.Code != http.StatusOK {
		t.Errorf("keeping its own SKU: status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	if got := decode[productBody](t, h.getProduct(green.ID, s.owner)); got.SKU != "tea-02" {
		t.Errorf("SKU = %q, want tea-02", got.SKU)
	}
}

// products R11: archived, never deleted; restoring brings it back to the Active list.
func TestArchivingAProductMovesItToTheArchivedList(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Stock: 4})

	h.updateProduct(tea.ID, map[string]any{"status": "archived"}, s.admin)

	if got := productNames(h.listProducts("", s.staff)); len(got) != 0 {
		t.Errorf("active = %v, want none", got)
	}
	archived := h.listProducts("status=archived", s.staff)
	if len(archived.Products) != 1 || archived.Products[0].Stock != 4 {
		t.Errorf("archived = %+v, want the Product with its stock as it was", archived.Products)
	}
	h.updateProduct(tea.ID, map[string]any{"status": "active"}, s.admin)
	if got := productNames(h.listProducts("", s.staff)); !slices.Equal(got, []string{"Thai tea"}) {
		t.Errorf("active = %v, want the restored Product", got)
	}
}

// products R9 at each boundary, as the List returns it.
func TestEveryProductCarriesItsStockState(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	for i, tc := range []struct {
		stock, minimum int32
		want           string
	}{
		{0, 0, "out-of-stock"},
		{0, 5, "out-of-stock"},
		{1, 0, "in-stock"},
		{1, 5, "low"},
		{5, 5, "low"},
		{6, 5, "in-stock"},
	} {
		h.seed(store.Product{Name: fmt.Sprintf("P%d", i), Sku: fmt.Sprintf("S%d", i), Stock: tc.stock, MinimumStock: tc.minimum})
	}

	page := h.listProducts("", s.staff)

	var got []string
	for _, p := range page.Products {
		got = append(got, p.StockState)
	}
	want := []string{"out-of-stock", "out-of-stock", "in-stock", "low", "low", "in-stock"}
	if !slices.Equal(got, want) {
		t.Errorf("states = %v, want %v", got, want)
	}
}

// products R15: every Role sees the same list.
func TestEveryRoleListsTheSameProducts(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	h.seed(store.Product{Name: "Thai tea", Sku: "TEA-01", Price: 12050, Stock: 1})
	h.seed(store.Product{Name: "Green tea", Sku: "TEA-02", Price: 9900, Stock: 8})

	owner := h.listProducts("", s.owner)
	for role, cookie := range map[string]*http.Cookie{"admin": s.admin, "staff": s.staff} {
		if got := h.listProducts("", cookie); !slices.Equal(got.Products, owner.Products) || got.Total != 2 {
			t.Errorf("%s sees %+v, the Owner %+v", role, got, owner)
		}
	}
	if owner.Products[0].Price != 9900 {
		t.Errorf("prices are not shown to the Owner: %+v", owner.Products)
	}
}

// products R13, R14: Active Products unless asked for Archived ones, sorted by name.
func TestListProductsShowsActiveProductsSortedByNameUnlessAskedForArchivedOnes(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	h.seed(store.Product{Name: "banana", Sku: "B"})
	h.seed(store.Product{Name: "Cherry", Sku: "C"})
	h.seed(store.Product{Name: "apple", Sku: "A"})
	h.seed(store.Product{Name: "Durian", Sku: "D", Status: "archived"})

	if got := productNames(h.listProducts("", s.staff)); !slices.Equal(got, []string{"apple", "banana", "Cherry"}) {
		t.Errorf("default = %v", got)
	}
	if got := productNames(h.listProducts("status=active", s.staff)); !slices.Equal(got, []string{"apple", "banana", "Cherry"}) {
		t.Errorf("active = %v", got)
	}
	if got := productNames(h.listProducts("status=archived", s.staff)); !slices.Equal(got, []string{"Durian"}) {
		t.Errorf("archived = %v", got)
	}
	assertProblem(t, h.do(http.MethodGet, "/products?status=all", nil, s.staff), http.StatusUnprocessableEntity, "")
}

// products R14.
func TestListProductsSearchesNameAndSKUWithoutRegardToLetterCase(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	h.seed(store.Product{Name: "Thai Tea", Sku: "DRK-1"})
	h.seed(store.Product{Name: "Rice", Sku: "tea-X"})
	h.seed(store.Product{Name: "Soda", Sku: "DRK-3"})
	h.seed(store.Product{Name: "ชาเขียว", Sku: "GRN"})
	h.seed(store.Product{Name: "Old tea", Sku: "OLD", Status: "archived"})

	for query, want := range map[string][]string{
		"search=TEA":                      {"Rice", "Thai Tea"},
		"search=drk":                      {"Soda", "Thai Tea"},
		"search=" + url.QueryEscape("ชา"): {"ชาเขียว"},
		"search=tea&status=archived":      {"Old tea"},
		"search=nothing":                  {},
		"search=":                         {"Rice", "Soda", "Thai Tea", "ชาเขียว"},
		"search=%20%20":                   {"Rice", "Soda", "Thai Tea", "ชาเขียว"},
	} {
		if got := productNames(h.listProducts(query, s.staff)); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", query, got, want)
		}
	}
}

// products R14: one or more stock states.
func TestListProductsFiltersByOneOrMoreStockStates(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	h.seed(store.Product{Name: "A out", Sku: "A", Stock: 0, MinimumStock: 2})
	h.seed(store.Product{Name: "B low", Sku: "B", Stock: 2, MinimumStock: 2})
	h.seed(store.Product{Name: "C in", Sku: "C", Stock: 3, MinimumStock: 2})
	h.seed(store.Product{Name: "D low archived", Sku: "D", Stock: 1, MinimumStock: 2, Status: "archived"})

	for query, want := range map[string][]string{
		"stock=low":                         {"B low"},
		"stock=out-of-stock":                {"A out"},
		"stock=in-stock":                    {"C in"},
		"stock=low&stock=out-of-stock":      {"A out", "B low"},
		"stock=low&status=archived":         {"D low archived"},
		"stock=in-stock&stock=low&search=b": {"B low"},
	} {
		page := h.listProducts(query, s.staff)
		if got := productNames(page); !slices.Equal(got, want) || page.Total != int64(len(want)) {
			t.Errorf("%s: %v (total %d), want %v", query, got, page.Total, want)
		}
	}
	assertProblem(t, h.do(http.MethodGet, "/products?stock=empty", nil, s.staff), http.StatusUnprocessableEntity, "")
}

func TestListProductsPagesAndCountsEveryMatch(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	for i := range 5 {
		h.seed(store.Product{Name: fmt.Sprintf("P%d", i), Sku: fmt.Sprintf("S%d", i)})
	}

	first := h.listProducts("limit=2", s.staff)
	second := h.listProducts("limit=2&offset=2", s.staff)
	last := h.listProducts("limit=2&offset=4", s.staff)
	past := h.listProducts("limit=2&offset=9", s.staff)

	for name, tc := range map[string]struct {
		got  productPage
		want []string
	}{
		"first": {first, []string{"P0", "P1"}}, "second": {second, []string{"P2", "P3"}},
		"last": {last, []string{"P4"}}, "past the end": {past, []string{}},
	} {
		if got := productNames(tc.got); !slices.Equal(got, tc.want) || tc.got.Total != 5 {
			t.Errorf("%s: %v (total %d), want %v of 5", name, got, tc.got.Total, tc.want)
		}
	}
	for _, query := range []string{"limit=0", "limit=101", "offset=-1", "limit=x"} {
		assertProblem(t, h.do(http.MethodGet, "/products?"+query, nil, s.staff), http.StatusUnprocessableEntity, "")
	}
}

func TestListProductsAnswersAnEmptyArrayWhenThereAreNone(t *testing.T) {
	h := newHarness(t)
	s := h.shop()

	rec := h.do(http.MethodGet, "/products", nil, s.staff)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"products":[]`) {
		t.Errorf("status %d, body %s; want 200 and an empty array", rec.Code, rec.Body)
	}
}

// A failing database is a 500 that tells nothing of it (ADR 0014, CODING_STANDARDS.md).
func TestProductOperationsAnswer500WithoutTheCauseWhenTheDatabaseFails(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Thai tea"})
	h.store.Err = fmt.Errorf("connection to host db.internal refused")

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"list":   h.do(http.MethodGet, "/products", nil, s.owner),
		"get":    h.getProduct(tea.ID, s.owner),
		"create": h.createProduct(validProduct, s.owner),
		"update": h.updateProduct(tea.ID, map[string]any{"name": "x"}, s.owner),
	} {
		assertProblem(t, rec, http.StatusInternalServerError, "")
		if strings.Contains(rec.Body.String(), "db.internal") {
			t.Errorf("%s: the 500 gives the cause away: %s", name, rec.Body)
		}
	}
}
