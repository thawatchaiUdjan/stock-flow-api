package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store/storetest"
)

// actorsHarness is a Shop with an Owner, a Staff User, a Product, one In-shop order made by
// the Staff User and one Online order, with their cookies.
type actorsHarness struct {
	*harness
	owner, staff   *http.Cookie
	inShop, online int64
	product        store.Product
}

func newActorsHarness(t *testing.T) *actorsHarness {
	t.Helper()
	h := newHarness(t)
	a := &actorsHarness{harness: h}
	h.addUser("owner@example.com", func(u *store.User) { u.Name = "Malee"; u.Role = "owner" })
	somchai := h.addUser("staff@example.com")
	a.owner, a.staff = h.session("owner@example.com"), h.session("staff@example.com")
	a.product = h.store.AddProduct(store.Product{Name: "Thai tea", Sku: "TEA-01", Unit: "box", Price: 4500, Stock: 20, Status: "active"})
	created, err := h.store.CreateOrder(context.Background(), store.CreateOrderParams{
		UserID: somchai.ID, Now: h.clock.Now, Customer: "Walk-in", Lines: []store.OrderLineParams{{ProductID: a.product.ID, Quantity: 2}},
	}, storetest.SystemBy)
	if err != nil {
		t.Fatal(err)
	}
	a.inShop = created.Number
	a.online = h.store.AddOnlineOrder(storetest.OnlineOrderSeed{
		Customer: "Somying", ProductID: a.product.ID, Quantity: 1, Price: 4500, At: h.clock.Now(),
	})
	return a
}

type orderListBody struct {
	Orders []struct {
		Number  int64  `json:"number"`
		Channel string `json:"channel"`
	} `json:"orders"`
	Total int64 `json:"total"`
}

type actorBody struct {
	Kind string `json:"kind"`
	ID   *int64 `json:"id"`
	Name string `json:"name"`
}

type orderV2Body struct {
	Number    int64     `json:"number"`
	Channel   string    `json:"channel"`
	CreatedBy actorBody `json:"createdBy"`
	Customer  string    `json:"customer"`
	Status    string    `json:"status"`
	Completed *struct {
		By actorBody `json:"by"`
	} `json:"completed"`
	Cancelled *struct {
		By actorBody `json:"by"`
	} `json:"cancelled"`
}

func numbers(list orderListBody) []int64 {
	var got []int64
	for _, o := range list.Orders {
		got = append(got, o.Number)
	}
	return got
}

// business ADR 0014, orders R15, R16, R21: /v2/orders lists both channels and filters by one.
func TestV2OrdersListBothChannelsAndFilterByChannel(t *testing.T) {
	a := newActorsHarness(t)

	for query, want := range map[string][]int64{
		"":                            {a.online, a.inShop},
		"?channel=online":             {a.online},
		"?channel=in-shop":            {a.inShop},
		"?channel=online&search=Walk": nil,
		"?search=Somying":             {a.online},
	} {
		rec := a.do(http.MethodGet, "/v2/orders"+query, nil, a.staff)
		if rec.Code != http.StatusOK {
			t.Fatalf("%q: status = %d; body %s", query, rec.Code, rec.Body)
		}
		list := decode[orderListBody](t, rec)
		if fmt.Sprint(numbers(list)) != fmt.Sprint(want) && !(len(want) == 0 && len(list.Orders) == 0) || list.Total != int64(len(want)) {
			t.Errorf("%q: numbers = %v, total %d, want %v", query, numbers(list), list.Total, want)
		}
		for _, o := range list.Orders {
			if o.Channel != "in-shop" && o.Channel != "online" {
				t.Errorf("%q: channel = %q", query, o.Channel)
			}
		}
	}
	if rec := a.do(http.MethodGet, "/v2/orders?channel=phone", nil, a.staff); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("unknown channel: status = %d, want 422", rec.Code)
	}
	if rec := a.do(http.MethodGet, "/v2/orders", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("no session: status = %d, want 401", rec.Code)
	}
}

// orders R17, business ADR 0014: the detail names who acted as a User, a Customer or the system.
func TestV2OrderDetailNamesWhoActed(t *testing.T) {
	a := newActorsHarness(t)

	inShop := decode[orderV2Body](t, a.do(http.MethodGet, fmt.Sprintf("/v2/orders/%d", a.inShop), nil, a.staff))
	online := decode[orderV2Body](t, a.do(http.MethodGet, fmt.Sprintf("/v2/orders/%d", a.online), nil, a.staff))

	if inShop.Channel != "in-shop" || inShop.CreatedBy.Kind != "user" || inShop.CreatedBy.ID == nil || inShop.CreatedBy.Name != "Somchai" {
		t.Errorf("In-shop order = %+v", inShop)
	}
	if online.Channel != "online" || online.CreatedBy.Kind != "customer" || online.CreatedBy.ID != nil || online.CreatedBy.Name != "Somying" || online.Customer != "Somying" {
		t.Errorf("Online order = %+v", online)
	}
}

// orders R11, R12: a User completes or cancels, and the system cancels; each is named.
func TestV2OrdersCompleteAndCancelNameTheActor(t *testing.T) {
	a := newActorsHarness(t)
	path := func(n int64, action string) string { return fmt.Sprintf("/v2/orders/%d/%s", n, action) }

	completed := decode[orderV2Body](t, a.do(http.MethodPost, path(a.online, "complete"), nil, a.staff))
	cancelled := decode[orderV2Body](t, a.do(http.MethodPost, path(a.inShop, "cancel"), nil, a.owner))

	if completed.Status != "completed" || completed.Completed == nil || completed.Completed.By.Kind != "user" || completed.Completed.By.Name != "Somchai" {
		t.Errorf("completed = %+v", completed)
	}
	if cancelled.Status != "cancelled" || cancelled.Cancelled == nil || cancelled.Cancelled.By.Kind != "user" || cancelled.Cancelled.By.Name != "Malee" {
		t.Errorf("cancelled = %+v", cancelled)
	}
	// The system's cancellation, as the Unpaid timeout of a later ticket makes it.
	system := a.store.AddOnlineOrder(storetest.OnlineOrderSeed{Customer: "Somsak", ProductID: a.product.ID, Quantity: 1, Price: 4500, At: a.clock.Now()})
	if _, err := a.store.CancelOrder(context.Background(), store.FinishOrderParams{Number: system, Now: a.clock.Now}, storetest.SystemBy); err != nil {
		t.Fatal(err)
	}
	got := decode[orderV2Body](t, a.do(http.MethodGet, fmt.Sprintf("/v2/orders/%d", system), nil, a.staff))
	if got.Cancelled == nil || got.Cancelled.By.Kind != "system" || got.Cancelled.By.ID != nil || got.Cancelled.By.Name != "System" {
		t.Errorf("system cancellation = %+v", got.Cancelled)
	}
	// Roles are the legacy operations': Staff do not cancel or delete.
	for _, req := range [][2]string{{http.MethodPost, path(a.online, "cancel")}, {http.MethodDelete, fmt.Sprintf("/v2/orders/%d", a.online)}} {
		if rec := a.do(req[0], req[1], nil, a.staff); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as Staff: status = %d, want 403", req[0], req[1], rec.Code)
		}
	}
	if rec := a.do(http.MethodDelete, fmt.Sprintf("/v2/orders/%d", a.online), nil, a.owner); rec.Code != http.StatusNoContent {
		t.Errorf("delete a completed Online order: status = %d, want 204; body %s", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodGet, fmt.Sprintf("/v2/orders/%d", a.online), nil, a.owner); rec.Code != http.StatusNotFound {
		t.Errorf("a deleted Order: status = %d, want 404", rec.Code)
	}
}

// business ADR 0014: the legacy operations cover In-shop orders only, and answer for an
// Online order as for no Order.
func TestLegacyOrdersCoverInShopOrdersOnly(t *testing.T) {
	a := newActorsHarness(t)

	list := decode[orderListBody](t, a.do(http.MethodGet, "/orders", nil, a.staff))
	if fmt.Sprint(numbers(list)) != fmt.Sprint([]int64{a.inShop}) || list.Total != 1 {
		t.Errorf("legacy list = %v, total %d, want only the In-shop order", numbers(list), list.Total)
	}
	path := fmt.Sprintf("/orders/%d", a.online)
	for _, req := range [][2]string{{http.MethodGet, path}, {http.MethodPost, path + "/complete"}, {http.MethodPost, path + "/cancel"}, {http.MethodDelete, path}} {
		if rec := a.do(req[0], req[1], nil, a.owner); rec.Code != http.StatusNotFound {
			t.Errorf("legacy %s %s: status = %d, want 404; body %s", req[0], req[1], rec.Code, rec.Body)
		}
	}
	// Nothing happened to it.
	if got := decode[orderV2Body](t, a.do(http.MethodGet, fmt.Sprintf("/v2/orders/%d", a.online), nil, a.owner)); got.Status != "pending" {
		t.Errorf("the Online order is %s, want pending", got.Status)
	}
}

// dashboard-and-reports: the Dashboard's newest Orders are In-shop; its figures count every Order.
func TestTheDashboardCountsEveryOrderButListsInShopOnes(t *testing.T) {
	a := newActorsHarness(t)
	somchai, _ := a.store.GetUserByEmail(context.Background(), "staff@example.com")
	a.store.AddOnlineOrder(storetest.OnlineOrderSeed{Customer: "Somsak", ProductID: a.product.ID, Quantity: 2, Price: 4500, CompletedBy: somchai.ID, At: a.clock.Now()})

	var dashboard struct {
		Sales           int64 `json:"sales"`
		OrdersCompleted int64 `json:"ordersCompleted"`
		PendingOrders   int64 `json:"pendingOrders"`
		NewestOrders    []struct {
			Number int64 `json:"number"`
		} `json:"newestOrders"`
	}
	rec := a.do(http.MethodGet, "/dashboard", nil, a.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body)
	}
	dashboard = decode[struct {
		Sales           int64 `json:"sales"`
		OrdersCompleted int64 `json:"ordersCompleted"`
		PendingOrders   int64 `json:"pendingOrders"`
		NewestOrders    []struct {
			Number int64 `json:"number"`
		} `json:"newestOrders"`
	}](t, rec)

	// Pending: the In-shop and the first Online order; Completed today: the second Online order.
	if dashboard.PendingOrders != 2 || dashboard.OrdersCompleted != 1 || dashboard.Sales != 9000 {
		t.Errorf("figures = %+v, want 2 Pending, 1 Completed, 9000 Sales", dashboard)
	}
	if len(dashboard.NewestOrders) != 1 || dashboard.NewestOrders[0].Number != a.inShop {
		t.Errorf("newest Orders = %+v, want only the In-shop order", dashboard.NewestOrders)
	}
}

type movementV2Body struct {
	Movements []struct {
		Kind  string    `json:"kind"`
		Actor actorBody `json:"actor"`
	} `json:"movements"`
	Total *int64 `json:"total"`
}

// stock-movements R2, R10: /v2/stock-movements shows every actor; the legacy list only Users.
func TestStockMovementsNameTheActorAndLegacyListsOnlyUsers(t *testing.T) {
	a := newActorsHarness(t)
	if _, err := a.store.CancelOrder(context.Background(), store.FinishOrderParams{Number: a.inShop, Now: a.clock.Now}, storetest.SystemBy); err != nil {
		t.Fatal(err)
	}

	v2 := decode[movementV2Body](t, a.do(http.MethodGet, "/v2/stock-movements?offset=0", nil, a.staff))
	legacy := decode[movementV2Body](t, a.do(http.MethodGet, "/stock-movements?offset=0", nil, a.staff))

	if len(v2.Movements) != 2 || *v2.Total != 2 {
		t.Fatalf("v2 movements = %+v", v2)
	}
	if m := v2.Movements[0]; m.Kind != "cancellation" || m.Actor.Kind != "system" || m.Actor.ID != nil || m.Actor.Name != "System" {
		t.Errorf("system movement = %+v", m)
	}
	if m := v2.Movements[1]; m.Kind != "order" || m.Actor.Kind != "user" || m.Actor.ID == nil || m.Actor.Name != "Somchai" {
		t.Errorf("User movement = %+v", m)
	}
	// A User cancelling the Online order makes a User movement the legacy list must not show:
	// it links to an Order the legacy operations cannot open.
	if rec := a.do(http.MethodPost, fmt.Sprintf("/v2/orders/%d/cancel", a.online), nil, a.owner); rec.Code != http.StatusOK {
		t.Fatalf("cancel the Online order: status = %d; body %s", rec.Code, rec.Body)
	}
	legacy = decode[movementV2Body](t, a.do(http.MethodGet, "/stock-movements?offset=0", nil, a.staff))
	if len(legacy.Movements) != 1 || legacy.Movements[0].Kind != "order" || *legacy.Total != 1 {
		t.Errorf("legacy movements = %+v, want only the User's", legacy)
	}
}

type loginLogV2Body struct {
	Entries []struct {
		App    string     `json:"app"`
		Email  string     `json:"email"`
		Result string     `json:"result"`
		Actor  *actorBody `json:"actor"`
	} `json:"entries"`
	Total *int64 `json:"total"`
}

// users-and-access R36: /v2/login-log shows both apps; the legacy log only StockFlow's.
func TestLoginLogV2ShowsBothAppsAndLegacyOnlyStockFlow(t *testing.T) {
	a := newActorsHarness(t)
	for i, result := range []string{store.ResultFailedLogin, store.ResultLogin} {
		entry := store.AddLoginLogEntryParams{
			At: a.clock.Now().Add(time.Duration(i) * time.Second), Email: "somying@example.com", Result: result, Ip: "203.0.113.7",
			App: pgtype.Text{String: store.AppStorefront, Valid: true}, CustomerID: pgtype.Int8{Int64: 7, Valid: i == 1},
		}
		if err := a.store.AddLoginLogEntry(context.Background(), entry); err != nil {
			t.Fatal(err)
		}
	}

	all := decode[loginLogV2Body](t, a.do(http.MethodGet, "/v2/login-log?offset=0", nil, a.owner))
	storefront := decode[loginLogV2Body](t, a.do(http.MethodGet, "/v2/login-log?offset=0&app=storefront", nil, a.owner))
	stockflow := decode[loginLogV2Body](t, a.do(http.MethodGet, "/v2/login-log?offset=0&app=stockflow", nil, a.owner))
	legacy := decode[loginLogEntriesBody](t, a.do(http.MethodGet, "/login-log?offset=0", nil, a.owner))

	if *all.Total != 4 || *storefront.Total != 2 || *stockflow.Total != 2 {
		t.Fatalf("totals: all %d, Storefront %d, StockFlow %d, want 4, 2, 2", *all.Total, *storefront.Total, *stockflow.Total)
	}
	if e := storefront.Entries[0]; e.App != "storefront" || e.Result != "login" || e.Actor == nil || e.Actor.Kind != "customer" || e.Actor.ID == nil || *e.Actor.ID != 7 {
		t.Errorf("Storefront login = %+v", e)
	}
	if e := storefront.Entries[1]; e.Result != "failed-login" || e.Actor != nil {
		t.Errorf("Storefront failed login = %+v, want no actor", e)
	}
	if e := stockflow.Entries[0]; e.App != "stockflow" || e.Actor == nil || e.Actor.Kind != "user" {
		t.Errorf("StockFlow entry = %+v", e)
	}
	if *legacy.Total != 2 {
		t.Errorf("legacy total = %d, want StockFlow's 2", *legacy.Total)
	}
	for _, e := range legacy.Entries {
		if e.Email == "somying@example.com" {
			t.Errorf("the legacy log shows a Storefront entry: %+v", e)
		}
	}
	// users-and-access R36: only the Owner reads the Login log.
	if rec := a.do(http.MethodGet, "/v2/login-log", nil, a.staff); rec.Code != http.StatusForbidden {
		t.Errorf("Staff: status = %d, want 403", rec.Code)
	}
}

type loginLogEntriesBody struct {
	Entries []struct {
		Email string `json:"email"`
	} `json:"entries"`
	Total *int64 `json:"total"`
}

// Contract 0.3.0.
func TestTheContractIsVersion030(t *testing.T) {
	h := newHarness(t)
	if got := h.contract.OpenAPI().Info.Version; got != "0.3.0" {
		t.Errorf("version = %q, want 0.3.0", got)
	}
}

// A v2 schema that left a field out would hide it from the generated client.
func TestTheV2SchemasPublishEveryField(t *testing.T) {
	h := newHarness(t)
	schemas := h.contract.OpenAPI().Components.Schemas.Map()
	for name, fields := range map[string][]string{
		"OrderSummaryV2":  {"id", "number", "channel", "createdAt", "customer", "lineCount", "total", "status"},
		"OrderV2":         {"id", "number", "channel", "createdAt", "createdBy", "customer", "note", "lines", "total", "status", "completed", "cancelled"},
		"StockMovementV2": {"id", "at", "kind", "product", "change", "stockAfter", "actor", "note", "order"},
		"LoginLogEntryV2": {"id", "at", "app", "email", "actor", "result", "ip", "userAgent"},
	} {
		schema := schemas[name]
		if schema == nil {
			t.Errorf("no schema %s", name)
			continue
		}
		for _, field := range fields {
			if schema.Properties[field] == nil {
				t.Errorf("%s has no %s", name, field)
			}
		}
	}
}
