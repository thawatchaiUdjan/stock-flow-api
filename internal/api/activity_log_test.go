package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

type activityEntry struct {
	ID    int64     `json:"id"`
	At    time.Time `json:"at"`
	Actor struct {
		Kind  string `json:"kind"`
		ID    *int64 `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
		Role  string `json:"role"`
	} `json:"actor"`
	Action  string `json:"action"`
	Subject struct {
		Kind string `json:"kind"`
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Ref  string `json:"ref"`
	} `json:"subject"`
	Changes []struct {
		Field  string `json:"field"`
		Before any    `json:"before"`
		After  any    `json:"after"`
	} `json:"changes"`
	IP        string `json:"ip"`
	UserAgent string `json:"userAgent"`
}

type activityPage struct {
	Entries []activityEntry `json:"entries"`
	Total   int64           `json:"total"`
}

func (h *harness) activityLog(query string, owner *http.Cookie) activityPage {
	h.t.Helper()
	rec := h.do(http.MethodGet, "/v2/activity-log"+query, nil, owner)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("list activity log %q: status = %d; body %s", query, rec.Code, rec.Body)
	}
	return decode[activityPage](h.t, rec)
}

// activityHarness is a Shop with an Owner who is logged in, and the Owner's session.
func activityHarness(t *testing.T) (*harness, *http.Cookie) {
	t.Helper()
	h := newHarness(t)
	h.addUser("owner@example.com", func(u *store.User) { u.Name = "Malee"; u.Role = "owner" })
	return h, h.session("owner@example.com")
}

func (h *harness) addProduct(cookie *http.Cookie, name, sku string) int64 {
	h.t.Helper()
	rec := h.do(http.MethodPost, "/products", map[string]any{"name": name, "sku": sku, "unit": "box", "price": 12000, "minimumStock": 2}, cookie)
	if rec.Code != http.StatusCreated {
		h.t.Fatalf("create product: status = %d; body %s", rec.Code, rec.Body)
	}
	return decode[struct {
		ID int64 `json:"id"`
	}](h.t, rec).ID
}

func (h *harness) patchProduct(cookie *http.Cookie, id int64, body map[string]any) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPatch, fmt.Sprintf("/products/%d", id), body, cookie)
}

// activity-log R1, R2, R4: each Product change is one entry, with who, what and where.
func TestEveryProductChangeIsOneActivityLogEntryWithWhoWhatAndWhere(t *testing.T) {
	h, owner := activityHarness(t)
	h.addUser("admin@example.com", func(u *store.User) { u.Name = "Somying"; u.Role = "admin" })
	admin := h.session("admin@example.com")

	call := func(method, path string, body string) {
		h.t.Helper()
		h.clock.Advance(time.Minute)
		rec := h.from("203.0.113.7:1", http.Header{"User-Agent": {"Firefox"}}, method, path, body, admin)
		if rec.Code >= 300 {
			h.t.Fatalf("%s %s: status = %d; body %s", method, path, rec.Code, rec.Body)
		}
	}
	call(http.MethodPost, "/products", `{"name":"Thai tea","sku":"TEA-01","unit":"box","price":12000}`)
	call(http.MethodPatch, "/products/1", `{"name":"Green tea","price":13000}`)
	call(http.MethodPatch, "/products/1", `{"status":"archived"}`)
	call(http.MethodPatch, "/products/1", `{"status":"active"}`)
	call(http.MethodPatch, "/products/1", `{"status":"archived"}`)
	call(http.MethodDelete, "/products/1", "")

	page := h.activityLog("?kind=product", owner)

	var actions []string
	for _, e := range page.Entries {
		actions = append(actions, e.Action)
	}
	// Newest first.
	want := []string{"deleted", "archived", "restored", "archived", "edited", "added"}
	if fmt.Sprint(actions) != fmt.Sprint(want) || page.Total != int64(len(want)) {
		t.Fatalf("actions = %v, total %d, want %v", actions, page.Total, want)
	}
	added, edited, deleted := page.Entries[5], page.Entries[4], page.Entries[0]
	if added.Actor.Kind != "user" || added.Actor.ID == nil || added.Actor.Name != "Somying" || added.Actor.Email != "admin@example.com" ||
		added.Actor.Role != "admin" || added.IP != "203.0.113.7" || added.UserAgent != "Firefox" ||
		added.Subject.Kind != "product" || added.Subject.Name != "Thai tea" || added.Subject.Ref != "TEA-01" || len(added.Changes) != 0 {
		t.Errorf("added = %+v", added)
	}
	if len(edited.Changes) != 2 || edited.Changes[0].Field != "name" || edited.Changes[0].Before != "Thai tea" || edited.Changes[0].After != "Green tea" ||
		edited.Changes[1].Field != "price" || edited.Changes[1].Before != float64(12000) || edited.Changes[1].After != float64(13000) {
		t.Errorf("edited changes = %+v, want only name and price, before and after", edited.Changes)
	}
	// An entry names the Product as it is after the change; the old name is in changes.
	if edited.Subject.Name != "Green tea" || edited.Subject.Ref != "TEA-01" {
		t.Errorf("edited subject = %+v, want the Product as it is after the change", edited.Subject)
	}
	// activity-log R11: a Deleted Product is still named, as it was.
	if deleted.Subject.Name != "Green tea" || deleted.Subject.Ref != "TEA-01" || deleted.Subject.ID != 1 {
		t.Errorf("deleted = %+v, want the Product's name and SKU kept", deleted.Subject)
	}
	if !edited.At.After(added.At) {
		t.Errorf("times: added %v, edited %v", added.At, edited.At)
	}
}

// activity-log R2: the actor is recorded as they were then.
func TestAnActivityEntryKeepsTheActorAsTheyWere(t *testing.T) {
	h, owner := activityHarness(t)
	admin := h.addUser("admin@example.com", func(u *store.User) { u.Name = "Somying"; u.Role = "admin" })
	cookie := h.session("admin@example.com")
	h.addProduct(cookie, "Thai tea", "TEA-01")

	rec := h.do(http.MethodPatch, fmt.Sprintf("/users/%d", admin.ID), map[string]any{"name": "Somying P.", "role": "staff"}, owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("change user: status = %d; body %s", rec.Code, rec.Body)
	}

	got := h.activityLog("?kind=product", owner).Entries[0].Actor
	if got.Name != "Somying" || got.Role != "admin" {
		t.Errorf("actor = %+v, want the name and Role the User had", got)
	}
}

// activity-log R3: a change that fails writes no entry, and an unchanged save is no edit.
func TestAFailedOrUnchangedProductChangeWritesNoEntry(t *testing.T) {
	h, owner := activityHarness(t)
	h.addProduct(owner, "Thai tea", "TEA-01")
	second := h.addProduct(owner, "Green tea", "TEA-02")
	before := h.activityLog("", owner).Total

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"SKU taken":          h.patchProduct(owner, second, map[string]any{"name": "Changed", "sku": "tea-01"}),
		"no such Product":    h.patchProduct(owner, 99, map[string]any{"name": "x"}),
		"delete when Active": h.do(http.MethodDelete, fmt.Sprintf("/products/%d", second), nil, owner),
		"create SKU taken":   h.do(http.MethodPost, "/products", map[string]any{"name": "Dup", "sku": "TEA-01", "unit": "box", "price": 1}, owner),
		"delete no Product":  h.do(http.MethodDelete, "/products/99", nil, owner),
	} {
		if rec.Code < 400 {
			t.Errorf("%s: status = %d, want a refusal", name, rec.Code)
		}
	}
	if rec := h.patchProduct(owner, second, map[string]any{"name": "Green tea", "price": 12000, "status": "active"}); rec.Code != http.StatusOK {
		t.Fatalf("unchanged save: status = %d; body %s", rec.Code, rec.Body)
	}

	if after := h.activityLog("", owner).Total; after != before {
		t.Errorf("entries = %d, want %d: nothing changed", after, before)
	}
}

// activity-log R3, slow in spirit: if the entry cannot be written the change is not saved.
func TestAProductChangeIsNotSavedWhenItsEntryCannotBeWritten(t *testing.T) {
	h, owner := activityHarness(t)
	id := h.addProduct(owner, "Thai tea", "TEA-01")
	h.store.ActivityErr = fmt.Errorf("activity_log is full")

	rec := h.patchProduct(owner, id, map[string]any{"name": "Green tea"})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body %s", rec.Code, rec.Body)
	}
	h.store.ActivityErr = nil
	got := h.do(http.MethodGet, fmt.Sprintf("/products/%d", id), nil, owner)
	if decode[struct {
		Name string `json:"name"`
	}](t, got).Name != "Thai tea" {
		t.Errorf("the Product changed although its entry failed: %s", got.Body)
	}
}

// users-and-access R11, activity-log R9: only the Owner.
func TestOnlyTheOwnerReachesTheActivityLog(t *testing.T) {
	h := newHarness(t)
	h.addUser("admin@example.com", func(u *store.User) { u.Role = "admin" })
	h.addUser("staff@example.com")

	assertProblemStatus := func(rec *httptest.ResponseRecorder, status int) {
		t.Helper()
		if rec.Code != status {
			t.Errorf("status = %d, want %d; body %s", rec.Code, status, rec.Body)
		}
	}
	assertProblemStatus(h.do(http.MethodGet, "/v2/activity-log", nil), http.StatusUnauthorized)
	assertProblemStatus(h.do(http.MethodGet, "/v2/activity-log", nil, h.session("admin@example.com")), http.StatusForbidden)
	assertProblemStatus(h.do(http.MethodGet, "/v2/activity-log", nil, h.session("staff@example.com")), http.StatusForbidden)
}

// activity-log R10, navigation-and-lists R5, R6, R8: filters, numbered pages, exact total.
func TestTheActivityLogFiltersAndPagesWithAnExactTotal(t *testing.T) {
	h, owner := activityHarness(t)
	admin := h.addUser("admin@example.com", func(u *store.User) { u.Role = "admin" })
	adminCookie := h.session("admin@example.com")
	// Bangkok is UTC+7: 2026-11-06 23:30 UTC is 2026-11-07 06:30 in the Shop.
	h.clock.Set(time.Date(2026, 11, 6, 16, 59, 0, 0, time.UTC)) // 23:59 on the 6th, Shop time
	h.addProduct(owner, "Thai tea", "TEA-01")
	h.clock.Set(time.Date(2026, 11, 6, 17, 0, 0, 0, time.UTC)) // 00:00 on the 7th
	for i := range 29 {
		h.addProduct(adminCookie, fmt.Sprintf("Tea %d", i), fmt.Sprintf("T-%d", i))
	}
	h.patchProduct(owner, 1, map[string]any{"price": 1})

	if page := h.activityLog("", owner); len(page.Entries) != 25 || page.Total != 31 {
		t.Errorf("default page = %d entries, total %d, want 25 and 31", len(page.Entries), page.Total)
	}
	for _, size := range []int{10, 50} {
		if page := h.activityLog(fmt.Sprintf("?limit=%d", size), owner); len(page.Entries) != min(size, 31) || page.Total != 31 {
			t.Errorf("limit %d = %d entries, total %d", size, len(page.Entries), page.Total)
		}
	}
	// Numbered pages are consecutive and newest first.
	first, second := h.activityLog("?limit=10&offset=0", owner), h.activityLog("?limit=10&offset=10", owner)
	if first.Entries[9].ID-1 != second.Entries[0].ID || first.Entries[0].ID != 31 {
		t.Errorf("pages are not consecutive: %d..%d then %d", first.Entries[0].ID, first.Entries[9].ID, second.Entries[0].ID)
	}
	if page := h.activityLog("?limit=10&offset=500", owner); len(page.Entries) != 0 || page.Total != 31 {
		t.Errorf("out of range = %d entries, total %d, want none and 31", len(page.Entries), page.Total)
	}
	for query, want := range map[string]int64{
		"?actorKind=user&actorId=" + fmt.Sprint(admin.ID): 29,
		"?actorKind=system":              0,
		"?kind=product&action=edited":    1,
		"?kind=user":                     0,
		"?action=added":                  30,
		"?from=2026-11-07":               30, // the entry at 23:59 on the 6th is out
		"?to=2026-11-06":                 1,
		"?from=2026-11-06&to=2026-11-07": 31,
		"?from=2026-11-08":               0,
	} {
		if page := h.activityLog(query, owner); page.Total != want {
			t.Errorf("%s: total = %d, want %d", query, page.Total, want)
		}
	}
	for _, bad := range []string{"?limit=100", "?limit=7", "?offset=-1", "?from=2026-11-07&to=2026-11-06", "?kind=widget"} {
		if rec := h.do(http.MethodGet, "/v2/activity-log"+bad, nil, owner); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d, want 422", bad, rec.Code)
		}
	}
}

// ADR 0019: the operation says what it can answer.
func TestTheActivityLogOperationDeclaresItsErrors(t *testing.T) {
	op := operations(t)["list-activity-log"]
	for _, status := range []string{"401", "403", "422"} {
		if op.Responses[status] == nil {
			t.Errorf("list-activity-log does not declare %s", status)
		}
	}
}
