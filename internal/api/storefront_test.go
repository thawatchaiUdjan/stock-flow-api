package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store/storetest"
)

type catalogueBody struct {
	Products []map[string]any `json:"products"`
	Total    int64            `json:"total"`
}

func (h *harness) catalogue(query string, cookies ...*http.Cookie) catalogueBody {
	h.t.Helper()
	rec := h.do(http.MethodGet, "/v2/storefront/products"+query, nil, cookies...)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("catalogue %q: status = %d; body %s", query, rec.Code, rec.Body)
	}
	return decode[catalogueBody](h.t, rec)
}

func names(c catalogueBody) []string {
	got := []string{}
	for _, p := range c.Products {
		got = append(got, p["name"].(string))
	}
	return got
}

func (h *harness) seedProduct(name, sku string, stock, minimum int32, status string) store.Product {
	return h.store.AddProduct(store.Product{Name: name, Sku: sku, Unit: "box", Price: 4500, Stock: stock, MinimumStock: minimum, Status: status})
}

// storefront R2, R3: anyone browses; only Active Products, sorted by name.
func TestTheCatalogueIsPublicAndListsActiveProductsByName(t *testing.T) {
	h := newHarness(t)
	h.seedProduct("Pen", "PEN-01", 5, 1, "active")
	h.seedProduct("Cup", "CUP-01", 5, 1, "active")
	h.seedProduct("Old tea", "OLD-01", 5, 1, "archived")
	deleted := h.seedProduct("Gone tea", "GONE-1", 5, 1, "archived")
	if err := h.store.DeleteProduct(context.Background(), store.DeleteParams{ID: deleted.ID, UserID: 1, At: h.clock.Now()}, storetest.SystemBy); err != nil {
		t.Fatal(err)
	}

	visitor := h.catalogue("")
	h.addUser("staff@example.com")
	loggedIn := h.catalogue("", h.session("staff@example.com"))

	if fmt.Sprint(names(visitor)) != "[Cup Pen]" || visitor.Total != 2 || fmt.Sprint(names(loggedIn)) != "[Cup Pen]" {
		t.Errorf("catalogue = %v (total %d), logged in %v, want [Cup Pen]", names(visitor), visitor.Total, names(loggedIn))
	}
}

// storefront R3: searched by name or SKU, in any letter case; a search that matches only
// Archived or Deleted Products is a real empty page.
func TestTheCatalogueSearchesByNameOrSKU(t *testing.T) {
	h := newHarness(t)
	h.seedProduct("Thai tea", "TEA-01", 5, 1, "active")
	h.seedProduct("Cup", "CUP-01", 5, 1, "active")
	h.seedProduct("Archived pen", "PEN-01", 5, 1, "archived")
	gone := h.seedProduct("Deleted mug", "MUG-01", 5, 1, "archived")
	if err := h.store.DeleteProduct(context.Background(), store.DeleteParams{ID: gone.ID, UserID: 1, At: h.clock.Now()}, storetest.SystemBy); err != nil {
		t.Fatal(err)
	}

	for query, want := range map[string]string{
		"?search=THAI":    "[Thai tea]",
		"?search=cup-01":  "[Cup]",
		"?search=":        "[Cup Thai tea]",
		"?search=pen":     "[]",
		"?search=mug":     "[]",
		"?search=nothing": "[]",
	} {
		got := h.catalogue(query)
		if fmt.Sprint(names(got)) != want || got.Total != int64(len(names(got))) {
			t.Errorf("%s: %v (total %d), want %s", query, names(got), got.Total, want)
		}
	}
}

// storefront R3: one bounded page at a time, with an exact total.
func TestTheCatalogueIsBoundedAndPaged(t *testing.T) {
	h := newHarness(t)
	for i := range 30 {
		h.seedProduct(fmt.Sprintf("Tea %02d", i), fmt.Sprintf("T-%02d", i), 5, 1, "active")
	}

	if got := h.catalogue(""); len(got.Products) != 25 || got.Total != 30 {
		t.Errorf("default page = %d products, total %d, want 25 and 30", len(got.Products), got.Total)
	}
	if got := h.catalogue("?limit=10&offset=20"); fmt.Sprint(names(got)[0]) != "Tea 20" || len(got.Products) != 10 || got.Total != 30 {
		t.Errorf("third page = %v (total %d)", names(got), got.Total)
	}
	if got := h.catalogue("?limit=10&offset=500"); len(got.Products) != 0 || got.Total != 30 {
		t.Errorf("out of range = %d products, total %d, want none and 30", len(got.Products), got.Total)
	}
	for _, bad := range []string{"?limit=0", "?limit=51", "?offset=-1"} {
		if rec := h.do(http.MethodGet, "/v2/storefront/products"+bad, nil); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d, want 422", bad, rec.Code)
		}
	}
}

// storefront R4, R18, products R9: availability, and no other stock figure.
func TestTheCatalogueShowsAvailabilityAndNoOtherStockFigure(t *testing.T) {
	h := newHarness(t)
	h.seedProduct("A in stock", "A-1", 50, 5, "active")
	h.seedProduct("B low", "B-1", 3, 5, "active")
	h.seedProduct("C at the minimum", "C-1", 5, 5, "active")
	h.seedProduct("D out", "D-1", 0, 5, "active")

	got := h.catalogue("").Products

	type want struct {
		availability string
		left         any
	}
	for i, w := range []want{{"in-stock", nil}, {"low", float64(3)}, {"low", float64(5)}, {"out-of-stock", nil}} {
		p := got[i]
		if p["availability"] != w.availability || p["left"] != w.left {
			t.Errorf("%v: availability %v, left %v, want %v and %v", p["name"], p["availability"], p["left"], w.availability, w.left)
		}
		for _, secret := range []string{"stock", "minimumStock", "stockState", "status"} {
			if _, ok := p[secret]; ok {
				t.Errorf("%v shows %s: the Storefront shows no other stock figure", p["name"], secret)
			}
		}
		if p["price"] != float64(4500) || p["sku"] == nil || p["unit"] != "box" {
			t.Errorf("%v = %v, want its price, SKU and unit", p["name"], p)
		}
	}
	// A Product that was Low and has been restocked is In stock: the count follows the stock.
	h.store.AddProduct(store.Product{Name: "E restocked", Sku: "E-1", Unit: "box", Price: 1, Stock: 6, MinimumStock: 5, Status: "active"})
	if p := h.catalogue("?search=E-1").Products[0]; p["availability"] != "in-stock" || p["left"] != nil {
		t.Errorf("restocked = %v", p)
	}
}
