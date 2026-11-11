package api_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/api"
)

func operations(t *testing.T) map[string]*huma.Operation {
	t.Helper()
	_, contract := api.New(api.Deps{})
	found := map[string]*huma.Operation{}
	for _, item := range contract.OpenAPI().Paths {
		for _, op := range []*huma.Operation{item.Get, item.Put, item.Post, item.Delete, item.Patch} {
			if op != nil {
				found[op.OperationID] = op
			}
		}
	}
	return found
}

func TestTheContractPublishesEveryOperationOfLoggingIn(t *testing.T) {
	ops := operations(t)

	for id, path := range map[string]string{
		"create-session":   "/login",
		"end-session":      "/logout",
		"get-current-user": "/me",
		"read-setup-link":  "/setup-link/read",
		"set-password":     "/setup-link/password",
	} {
		op, ok := ops[id]
		if !ok {
			t.Errorf("no operation %q", id)
			continue
		}
		if op.Path != path {
			t.Errorf("%s: path = %q, want %q", id, op.Path, path)
		}
	}
}

// users-and-access R4: nobody can sign up. What needs no session is this list and no
// more, so an operation that makes a User can't be reached without logging in.
func TestOnlyTheseOperationsNeedNoSession(t *testing.T) {
	public := []string{"create-session", "get-health", "read-setup-link", "request-first-time-code", "request-password-reset", "reset-email-password", "set-email-password", "set-password", "verify-first-time-code"}

	var got []string
	for id, op := range operations(t) {
		if len(op.Security) == 0 {
			got = append(got, id)
		}
	}

	slices.Sort(got)
	if !slices.Equal(got, public) {
		t.Errorf("operations with no session = %v, want %v", got, public)
	}
}

// users-and-access R17: a User is never deleted, so nothing in the API deletes.
func TestNoOperationDeletes(t *testing.T) {
	for id, op := range operations(t) {
		if op.Method == http.MethodDelete {
			t.Errorf("%s is a DELETE", id)
		}
	}
}

func TestTheContractDeclaresTheSessionCookie(t *testing.T) {
	_, contract := api.New(api.Deps{})

	scheme := contract.OpenAPI().Components.SecuritySchemes["session"]

	if scheme == nil || scheme.Type != "apiKey" || scheme.In != "cookie" || scheme.Name != "session" {
		t.Errorf("scheme = %+v, want an apiKey in the session cookie", scheme)
	}
	for _, id := range []string{"end-session", "get-current-user", "list-users", "create-user", "update-user", "reset-password", "create-email-user"} {
		op := operations(t)[id]
		if len(op.Security) != 1 {
			t.Errorf("%s: security = %v, want the session scheme", id, op.Security)
		}
		for _, param := range op.Parameters {
			if param.Name == "session" {
				t.Errorf("%s: the session cookie is a client parameter", id)
			}
		}
	}
}

// The token stays out of paths and query strings, and so out of request logs.
func TestASetupLinkTokenTravelsOnlyInRequestBodies(t *testing.T) {
	ops := operations(t)

	for _, id := range []string{"read-setup-link", "set-password", "verify-first-time-code", "set-email-password", "reset-email-password"} {
		op := ops[id]
		if op.Method != http.MethodPost || len(op.Parameters) != 0 || strings.Contains(op.Path, "{") {
			t.Errorf("%s: %s %s with parameters %v", id, op.Method, op.Path, op.Parameters)
		}
		if op.RequestBody == nil || op.RequestBody.Content["application/json"] == nil {
			t.Errorf("%s has no JSON body", id)
		}
	}
}

// ADR 0014: every status an operation can return is declared on it.
func TestEveryOperationDeclaresItsErrors(t *testing.T) {
	ops := operations(t)

	for id, statuses := range map[string][]string{
		"create-session":          {"401"},
		"end-session":             {"401"},
		"get-current-user":        {"401"},
		"read-setup-link":         {"410"},
		"set-password":            {"410", "422"},
		"list-users":              {"401", "403"},
		"create-user":             {"401", "403", "409", "422"},
		"update-user":             {"401", "403", "404", "422"},
		"reset-password":          {"401", "403", "404", "409"},
		"create-email-user":       {"401", "403", "409", "422", "500", "503"},
		"request-first-time-code": {"422", "500", "503"},
		"verify-first-time-code":  {"410", "422", "500", "503"},
		"set-email-password":      {"410", "422", "500", "503"},
		"request-password-reset":  {"422", "500", "503"},
		"reset-email-password":    {"410", "422", "500", "503"},
	} {
		for _, status := range statuses {
			if ops[id].Responses[status] == nil {
				t.Errorf("%s does not declare %s; it has %v", id, status, slices.Sorted(maps.Keys(ops[id].Responses)))
			}
		}
	}
}

// ADR 0019: each operation says which codes it can answer.
func TestEveryOperationNamesItsErrorCodes(t *testing.T) {
	ops := operations(t)

	for id, code := range map[string]string{
		"create-session":          "login-failed",
		"end-session":             "session-ended",
		"get-current-user":        "session-ended",
		"read-setup-link":         "setup-link-invalid",
		"set-password":            "setup-link-invalid",
		"list-users":              "session-ended",
		"create-user":             "email-taken",
		"update-user":             "target-is-owner",
		"reset-password":          "user-inactive",
		"create-email-user":       "email-taken",
		"request-first-time-code": "email-auth-unavailable",
		"verify-first-time-code":  "first-time-code-invalid",
		"set-email-password":      "password-setup-invalid",
		"request-password-reset":  "email-auth-unavailable",
		"reset-email-password":    "password-reset-invalid",
	} {
		if !strings.Contains(ops[id].Description, "`"+code+"`") {
			t.Errorf("%s: description %q does not name %s", id, ops[id].Description, code)
		}
	}
}

// ADR 0019: the contract is the list of codes stock-flow-web checks against.
func TestTheContractPublishesEveryErrorCode(t *testing.T) {
	_, contract := api.New(api.Deps{})

	model := contract.OpenAPI().Components.Schemas.Map()["ErrorModel"]

	typ := model.Properties["type"]
	want := []any{"about:blank", "login-failed", "session-ended", "setup-link-invalid", "email-taken", "target-is-owner", "user-inactive", "sku-taken", "insufficient-stock", "product-archived", "order-not-pending", "email-auth-unavailable", "first-time-code-invalid", "password-setup-invalid", "password-reset-invalid"}
	if !slices.Equal(typ.Enum, want) {
		t.Errorf("type enum = %v, want %v", typ.Enum, want)
	}
	for _, code := range want[1:] {
		if !strings.Contains(typ.Description, code.(string)+": ") {
			t.Errorf("type description does not explain %q: %q", code, typ.Description)
		}
	}
}

// The served contract and openapi.yaml must carry the list too, not only the Go value.
func TestTheServedContractCarriesTheErrorCodes(t *testing.T) {
	h := newHarness(t)

	rec := h.do(http.MethodGet, "/openapi.yaml", nil)

	if !strings.Contains(rec.Body.String(), "session-ended") {
		t.Errorf("/openapi.yaml has no session-ended:\n%s", rec.Body)
	}
}

// users-and-access R8.
func TestTheContractListsTheThreeRoles(t *testing.T) {
	_, contract := api.New(api.Deps{})

	role := contract.OpenAPI().Components.Schemas.Map()["CurrentUser"].Properties["role"]

	if want := []any{"owner", "admin", "staff"}; !slices.Equal(role.Enum, want) {
		t.Errorf("role enum = %v, want %v", role.Enum, want)
	}
}

// users-and-access R10, R33.
func TestTheContractPublishesListUsersAndCreateUser(t *testing.T) {
	ops := operations(t)

	for id, want := range map[string][2]string{
		"list-users":  {http.MethodGet, "/users"},
		"create-user": {http.MethodPost, "/users"},
	} {
		op, ok := ops[id]
		if !ok {
			t.Errorf("no operation %q", id)
			continue
		}
		if op.Method != want[0] || op.Path != want[1] {
			t.Errorf("%s: %s %s, want %s %s", id, op.Method, op.Path, want[0], want[1])
		}
		// users-and-access R10: the contract says who may call it.
		if !strings.Contains(op.Description, "Owner") {
			t.Errorf("%s: description %q does not say it is for the Owner", id, op.Description)
		}
	}
}

// users-and-access R9, R10: the contract offers Admin and Staff to create, never Owner.
func TestTheContractOffersOnlyAdminAndStaffToCreateAUser(t *testing.T) {
	_, contract := api.New(api.Deps{})

	body := contract.OpenAPI().Components.Schemas.Map()["CreateUserBody"]

	if want := []any{"admin", "staff"}; !slices.Equal(body.Properties["role"].Enum, want) {
		t.Errorf("role enum = %v, want %v", body.Properties["role"].Enum, want)
	}
}

// users-and-access R16, R29, R31, R35.
func TestTheContractPublishesUpdateUserAndResetPassword(t *testing.T) {
	ops := operations(t)

	for id, want := range map[string][2]string{
		"update-user":    {http.MethodPatch, "/users/{id}"},
		"reset-password": {http.MethodPost, "/users/{id}/reset-password"},
	} {
		op, ok := ops[id]
		if !ok {
			t.Errorf("no operation %q", id)
			continue
		}
		if op.Method != want[0] || op.Path != want[1] {
			t.Errorf("%s: %s %s, want %s %s", id, op.Method, op.Path, want[0], want[1])
		}
		for _, says := range []string{"Owner only", "`target-is-owner`", "`session-ended`"} {
			if !strings.Contains(op.Description, says) {
				t.Errorf("%s: description %q does not say %s", id, op.Description, says)
			}
		}
	}
}

// users-and-access R9, R15, R35: the contract offers a name, Admin or Staff and a status,
// each on its own, and neither an email nor the Role Owner.
func TestTheContractOffersOnlyWhatUpdateUserCanChange(t *testing.T) {
	_, contract := api.New(api.Deps{})

	body := contract.OpenAPI().Components.Schemas.Map()["UpdateUserBody"]

	if got, want := slices.Sorted(maps.Keys(body.Properties)), []string{"$schema", "name", "role", "status"}; !slices.Equal(got, want) {
		t.Errorf("properties = %v, want %v", got, want)
	}
	if len(body.Required) != 0 {
		t.Errorf("required = %v, want none: any of them will do", body.Required)
	}
	if want := []any{"admin", "staff"}; !slices.Equal(body.Properties["role"].Enum, want) {
		t.Errorf("role enum = %v, want %v", body.Properties["role"].Enum, want)
	}
	if want := []any{"active", "inactive"}; !slices.Equal(body.Properties["status"].Enum, want) {
		t.Errorf("status enum = %v, want %v", body.Properties["status"].Enum, want)
	}
}

// An empty list is [], never null, so the web client types it as an array.
func TestTheContractListsUsersAsAnArrayThatIsNeverNull(t *testing.T) {
	_, contract := api.New(api.Deps{})

	users := contract.OpenAPI().Components.Schemas.Map()["ListUsersOutputBody"].Properties["users"]

	if users.Type != "array" || users.Nullable {
		t.Errorf("users: type %q, nullable %v, want a plain array", users.Type, users.Nullable)
	}
}

// The ticket's four operations, and who may call them (users-and-access R11).
func TestTheContractPublishesTheFourProductOperations(t *testing.T) {
	ops := operations(t)

	for id, want := range map[string][2]string{
		"list-products":  {http.MethodGet, "/products"},
		"get-product":    {http.MethodGet, "/products/{id}"},
		"create-product": {http.MethodPost, "/products"},
		"update-product": {http.MethodPatch, "/products/{id}"},
	} {
		op, ok := ops[id]
		if !ok {
			t.Errorf("no operation %q", id)
			continue
		}
		if op.Method != want[0] || op.Path != want[1] {
			t.Errorf("%s: %s %s, want %s %s", id, op.Method, op.Path, want[0], want[1])
		}
		if len(op.Security) != 1 {
			t.Errorf("%s: security = %v, want the session scheme", id, op.Security)
		}
		for _, says := range []string{"`session-ended`"} {
			if !strings.Contains(op.Description, says) {
				t.Errorf("%s: description %q does not say %s", id, op.Description, says)
			}
		}
	}
	for _, id := range []string{"create-product", "update-product"} {
		if !strings.Contains(ops[id].Description, "Staff") || !strings.Contains(ops[id].Description, "`sku-taken`") {
			t.Errorf("%s: description %q does not say Staff are refused and when sku-taken", id, ops[id].Description)
		}
		for _, status := range []string{"401", "403", "409", "422"} {
			if ops[id].Responses[status] == nil {
				t.Errorf("%s does not declare %s", id, status)
			}
		}
	}
	for id, statuses := range map[string][]string{"list-products": {"401", "422"}, "get-product": {"401", "404", "422"}} {
		for _, status := range statuses {
			if ops[id].Responses[status] == nil {
				t.Errorf("%s does not declare %s", id, status)
			}
		}
		if ops[id].Responses["403"] != nil {
			t.Errorf("%s declares 403, but every Role may call it", id)
		}
	}
}

// Money is whole satang (context spec, Money): the contract has no floating-point amount.
func TestTheContractHasPricesInWholeSatang(t *testing.T) {
	_, contract := api.New(api.Deps{})
	schemas := contract.OpenAPI().Components.Schemas.Map()

	for name, price := range map[string]*huma.Schema{
		"Product":           schemas["Product"].Properties["price"],
		"CreateProductBody": schemas["CreateProductBody"].Properties["price"],
		"UpdateProductBody": schemas["UpdateProductBody"].Properties["price"],
	} {
		if price.Type != "integer" || price.Format != "int64" {
			t.Errorf("%s.price: type %q, format %q, want an int64 integer", name, price.Type, price.Format)
		}
	}
	if p := schemas["CreateProductBody"].Properties["price"]; *p.Minimum != 0 || *p.Maximum != 999999999 {
		t.Errorf("price bounds = %v to %v, want 0 to 999999999", *p.Minimum, *p.Maximum)
	}
}

// products R6, R7, R11: the contract offers no way to set stock; a new Product is Active
// and its status is changed by Update.
func TestTheContractOffersOnlyWhatCreateAndUpdateProductCanSet(t *testing.T) {
	_, contract := api.New(api.Deps{})
	schemas := contract.OpenAPI().Components.Schemas.Map()

	create, update := schemas["CreateProductBody"], schemas["UpdateProductBody"]

	if got, want := slices.Sorted(maps.Keys(create.Properties)), []string{"$schema", "minimumStock", "name", "price", "sku", "unit"}; !slices.Equal(got, want) {
		t.Errorf("create properties = %v, want %v", got, want)
	}
	if want := []string{"name", "price", "sku", "unit"}; !slices.Equal(slices.Sorted(slices.Values(create.Required)), want) {
		t.Errorf("create requires %v, want %v: the Minimum stock is 0 when left out", create.Required, want)
	}
	if got, want := slices.Sorted(maps.Keys(update.Properties)), []string{"$schema", "minimumStock", "name", "price", "sku", "status", "unit"}; !slices.Equal(got, want) {
		t.Errorf("update properties = %v, want %v", got, want)
	}
	if len(update.Required) != 0 {
		t.Errorf("update requires %v, want none: any of them will do", update.Required)
	}
	if want := []any{"active", "archived"}; !slices.Equal(update.Properties["status"].Enum, want) {
		t.Errorf("status enum = %v, want %v", update.Properties["status"].Enum, want)
	}
}

// products R9: the contract names the three states, and every Product carries one.
func TestTheContractListsTheThreeStockStates(t *testing.T) {
	_, contract := api.New(api.Deps{})
	schemas := contract.OpenAPI().Components.Schemas.Map()

	state := schemas["Product"].Properties["stockState"]

	if want := []any{"in-stock", "low", "out-of-stock"}; !slices.Equal(state.Enum, want) {
		t.Errorf("stockState enum = %v, want %v", state.Enum, want)
	}
	if !slices.Contains(schemas["Product"].Required, "stockState") {
		t.Errorf("stockState is not required in %v", schemas["Product"].Required)
	}
	var stock *huma.Param
	for _, param := range operations(t)["list-products"].Parameters {
		if param.Name == "stock" {
			stock = param
		}
	}
	if stock == nil || stock.Schema.Type != "array" || !slices.Equal(stock.Schema.Items.Enum, []any{"in-stock", "low", "out-of-stock"}) {
		t.Errorf("list-products stock parameter = %+v, want an array of the three states", stock)
	}
	if stock != nil && (stock.Explode == nil || !*stock.Explode) {
		t.Errorf("stock parameter explode = %v, want true: stock=low&stock=out-of-stock", stock.Explode)
	}
}

// Active unless asked otherwise (products R14), and an empty list is [] (never null).
func TestTheContractListsActiveProductsByDefaultAndNeverNull(t *testing.T) {
	_, contract := api.New(api.Deps{})
	var status *huma.Param
	for _, param := range operations(t)["list-products"].Parameters {
		if param.Name == "status" {
			status = param
		}
	}

	if status == nil || status.Schema.Default != "active" {
		t.Errorf("status parameter = %+v, want a default of active", status)
	}
	products := contract.OpenAPI().Components.Schemas.Map()["ListProductsOutputBody"].Properties["products"]
	if products.Type != "array" || products.Nullable {
		t.Errorf("products: type %q, nullable %v, want a plain array", products.Type, products.Nullable)
	}
}

func TestTheContractPublishesTheThreeStockMovementOperations(t *testing.T) {
	ops := operations(t)

	for id, want := range map[string]struct {
		method, path string
		statuses     []string
		codes        []string
	}{
		"list-stock-movements": {http.MethodGet, "/stock-movements", []string{"401", "422"}, nil},
		"receive-stock":        {http.MethodPost, "/stock-movements/receive", []string{"201", "401", "404", "409", "422"}, []string{"product-archived"}},
		"adjust-stock":         {http.MethodPost, "/stock-movements/adjust", []string{"201", "401", "403", "404", "409", "422"}, []string{"product-archived", "insufficient-stock"}},
	} {
		op, ok := ops[id]
		if !ok {
			t.Errorf("no operation %q", id)
			continue
		}
		if op.Method != want.method || op.Path != want.path || len(op.Security) != 1 {
			t.Errorf("%s: %s %s with security %v, want %s %s with the session scheme", id, op.Method, op.Path, op.Security, want.method, want.path)
		}
		for _, status := range want.statuses {
			if op.Responses[status] == nil {
				t.Errorf("%s does not declare %s; it has %v", id, status, slices.Sorted(maps.Keys(op.Responses)))
			}
		}
		for _, code := range append(want.codes, "session-ended") {
			if !strings.Contains(op.Description, "`"+code+"`") {
				t.Errorf("%s: description %q does not name %s", id, op.Description, code)
			}
		}
	}
	// stock-movements R4, R5: every Role receives; only the Owner and Admins adjust.
	if ops["receive-stock"].Responses["403"] != nil || ops["list-stock-movements"].Responses["403"] != nil {
		t.Error("receive-stock or list-stock-movements declares 403, but every Role may call them")
	}
	if !strings.Contains(ops["adjust-stock"].Description, "Staff") {
		t.Errorf("adjust-stock: description %q does not say Staff are refused", ops["adjust-stock"].Description)
	}
}

// stock-movements R8: a movement is never edited or deleted, so only GET and POST reach
// one, and the POSTs only make new ones.
func TestNoOperationEditsOrDeletesAStockMovement(t *testing.T) {
	for id, op := range operations(t) {
		if strings.HasPrefix(op.Path, "/stock-movements") && op.Method != http.MethodGet && op.Method != http.MethodPost {
			t.Errorf("%s is a %s on %s", id, op.Method, op.Path)
		}
	}
}

// stock-movements R3: all four kinds are published, though only two are made until Orders.
func TestTheContractListsTheFourKindsOfStockMovement(t *testing.T) {
	_, contract := api.New(api.Deps{})
	want := []any{"receive", "adjustment", "order", "cancellation"}

	movement := contract.OpenAPI().Components.Schemas.Map()["StockMovement"]

	if !slices.Equal(movement.Properties["kind"].Enum, want) {
		t.Errorf("kind enum = %v, want %v", movement.Properties["kind"].Enum, want)
	}
	for _, param := range operations(t)["list-stock-movements"].Parameters {
		if param.Name == "kind" && !slices.Equal(param.Schema.Enum, want) {
			t.Errorf("kind parameter enum = %v, want %v", param.Schema.Enum, want)
		}
	}
	// stock-movements R11: the Order, or null.
	order := movement.Properties["order"]
	if len(order.OneOf) != 2 || order.OneOf[1].Type != "null" || !slices.Contains(movement.Required, "order") {
		t.Errorf("order = %+v, required %v, want a required Order or null", order, movement.Required)
	}
}

// stock-movements R4, R5: the contract says a Receive is at least 1, an Adjustment is not
// 0 and has a note.
func TestTheContractStatesWhatEachStockChangeNeeds(t *testing.T) {
	_, contract := api.New(api.Deps{})
	schemas := contract.OpenAPI().Components.Schemas.Map()

	receive, adjust := schemas["ReceiveStockBody"], schemas["AdjustStockBody"]

	if q := receive.Properties["quantity"]; q.Type != "integer" || q.Minimum == nil || *q.Minimum != 1 {
		t.Errorf("quantity = %+v, want an integer of at least 1", q)
	}
	if slices.Contains(receive.Required, "note") || !slices.Contains(adjust.Required, "note") {
		t.Errorf("required: receive %v, adjust %v; want a note only for adjust", receive.Required, adjust.Required)
	}
	if c := adjust.Properties["change"]; c.Type != "integer" || c.Not == nil || !slices.Equal(c.Not.Enum, []any{0}) {
		t.Errorf("change = %+v, want an integer that is not 0", c)
	}
}

// stock-movements R6: insufficient-stock carries each Product's id and stock.
func TestTheContractCarriesTheStockOfInsufficientStock(t *testing.T) {
	_, contract := api.New(api.Deps{})
	schemas := contract.OpenAPI().Components.Schemas

	products := schemas.Map()["ErrorModel"].Properties["products"]

	if products == nil || products.Type != "array" || products.Items == nil {
		t.Fatalf("ErrorModel.products = %+v, want an array", products)
	}
	item := products.Items
	if item.Ref != "" {
		item = schemas.SchemaFromRef(item.Ref)
	}
	if got := slices.Sorted(maps.Keys(item.Properties)); !slices.Equal(got, []string{"id", "stock"}) {
		t.Errorf("products items = %v, want id and stock", got)
	}
	if !strings.Contains(products.Description, "insufficient-stock") {
		t.Errorf("description %q does not say when it is there", products.Description)
	}
}

// The ticket's five operations, who may call them, and what each can answer (orders R5,
// R11, R12, R17, users-and-access R11).
func TestTheContractPublishesTheFiveOrderOperations(t *testing.T) {
	ops := operations(t)

	for id, want := range map[string]struct {
		method, path string
		statuses     []string
		codes        []string
	}{
		"list-orders":    {http.MethodGet, "/orders", []string{"200", "401", "422"}, nil},
		"get-order":      {http.MethodGet, "/orders/{number}", []string{"200", "401", "404", "422"}, nil},
		"create-order":   {http.MethodPost, "/orders", []string{"201", "401", "404", "409", "422"}, []string{"insufficient-stock", "product-archived"}},
		"complete-order": {http.MethodPost, "/orders/{number}/complete", []string{"200", "401", "404", "409", "422"}, []string{"order-not-pending"}},
		"cancel-order":   {http.MethodPost, "/orders/{number}/cancel", []string{"200", "401", "403", "404", "409", "422"}, []string{"order-not-pending"}},
	} {
		op, ok := ops[id]
		if !ok {
			t.Errorf("no operation %q", id)
			continue
		}
		if op.Method != want.method || op.Path != want.path || len(op.Security) != 1 {
			t.Errorf("%s: %s %s with security %v, want %s %s with the session scheme", id, op.Method, op.Path, op.Security, want.method, want.path)
		}
		for _, status := range want.statuses {
			if op.Responses[status] == nil {
				t.Errorf("%s does not declare %s; it has %v", id, status, slices.Sorted(maps.Keys(op.Responses)))
			}
		}
		for _, code := range append(want.codes, "session-ended") {
			if !strings.Contains(op.Description, "`"+code+"`") {
				t.Errorf("%s: description %q does not name %s", id, op.Description, code)
			}
		}
		// orders R5, R11, R18: only Cancel is refused to anyone.
		if id != "cancel-order" && op.Responses["403"] != nil {
			t.Errorf("%s declares 403, but every Role may call it", id)
		}
	}
	if !strings.Contains(ops["cancel-order"].Description, "Staff") {
		t.Errorf("cancel-order: description %q does not say Staff are refused", ops["cancel-order"].Description)
	}
}

// orders R14: an Order is never edited, so nothing but its own Complete and Cancel
// changes one, and nothing deletes one.
func TestNoOperationEditsAnOrder(t *testing.T) {
	for id, op := range operations(t) {
		if !strings.HasPrefix(op.Path, "/orders") || op.Method == http.MethodGet {
			continue
		}
		if op.Method != http.MethodPost || op.Path != "/orders" && !strings.HasSuffix(op.Path, "/complete") && !strings.HasSuffix(op.Path, "/cancel") {
			t.Errorf("%s is a %s on %s", id, op.Method, op.Path)
		}
	}
}

// orders R1, R3, R4, R10: the contract says what an Order holds, in whole satang, and
// what Create Order takes.
func TestTheContractDescribesAnOrder(t *testing.T) {
	_, contract := api.New(api.Deps{})
	schemas := contract.OpenAPI().Components.Schemas.Map()

	o, body := schemas["Order"], schemas["CreateOrderBody"]

	if o == nil || body == nil {
		t.Fatalf("Order = %v, CreateOrderBody = %v", o, body)
	}
	if want := []any{"pending", "completed", "cancelled"}; !slices.Equal(o.Properties["status"].Enum, want) {
		t.Errorf("status enum = %v, want %v", o.Properties["status"].Enum, want)
	}
	for _, name := range []string{"completed", "cancelled"} {
		f := o.Properties[name]
		if len(f.OneOf) != 2 || f.OneOf[1].Type != "null" || !slices.Contains(o.Required, name) {
			t.Errorf("%s = %+v, want a required object or null", name, f)
		}
	}
	line := schemas["OrderLine"]
	for name, s := range map[string]*huma.Schema{"total": o.Properties["total"], "price": line.Properties["price"], "amount": line.Properties["amount"]} {
		if s.Type != "integer" || s.Format != "int64" {
			t.Errorf("%s: type %q, format %q, want whole satang as an int64", name, s.Type, s.Format)
		}
	}
	if got, want := slices.Sorted(maps.Keys(body.Properties)), []string{"$schema", "customer", "lines", "note"}; !slices.Equal(got, want) {
		t.Errorf("create properties = %v, want %v", got, want)
	}
	if !slices.Equal(body.Required, []string{"lines"}) || *body.Properties["lines"].MinItems != 1 {
		t.Errorf("create requires %v, lines min %v; want only lines, at least one", body.Required, body.Properties["lines"].MinItems)
	}
	if *body.Properties["customer"].MaxLength != 120 || *body.Properties["note"].MaxLength != 200 {
		t.Error("customer and note are not limited to 120 and 200")
	}
	if q := schemas["CreateOrderLine"].Properties["quantity"]; q.Type != "integer" || *q.Minimum != 1 {
		t.Errorf("quantity = %+v, want an integer of at least 1", q)
	}
}

// dashboard-and-reports R3-R8, users-and-access R11: Get Dashboard and Get Sales report
// are for the Owner and Admins, and the report's range can be refused.
func TestTheContractPublishesTheDashboardAndTheSalesReport(t *testing.T) {
	ops := operations(t)

	for id, want := range map[string]struct {
		path     string
		statuses []string
	}{
		"get-dashboard":    {"/dashboard", []string{"200", "401", "403"}},
		"get-sales-report": {"/reports/sales", []string{"200", "401", "403", "422"}},
	} {
		op, ok := ops[id]
		if !ok {
			t.Errorf("no operation %q", id)
			continue
		}
		if op.Method != http.MethodGet || op.Path != want.path || len(op.Security) != 1 {
			t.Errorf("%s: %s %s with security %v, want GET %s with the session scheme", id, op.Method, op.Path, op.Security, want.path)
		}
		for _, status := range want.statuses {
			if op.Responses[status] == nil {
				t.Errorf("%s does not declare %s; it has %v", id, status, slices.Sorted(maps.Keys(op.Responses)))
			}
		}
		if !strings.Contains(op.Description, "Staff") || !strings.Contains(op.Description, "`session-ended`") {
			t.Errorf("%s: description %q does not say Staff are refused and a session is needed", id, op.Description)
		}
	}
	if ops["get-dashboard"].Responses["422"] != nil {
		t.Errorf("get-dashboard takes no input, but declares 422")
	}
}

// contractPatterns finds every "pattern" in the published contract, with where it sits.
func contractPatterns(t *testing.T) map[string]string {
	t.Helper()
	_, contract := api.New(api.Deps{})
	raw, err := json.Marshal(contract.OpenAPI())
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	var walk func(path string, node any)
	walk = func(path string, node any) {
		switch n := node.(type) {
		case map[string]any:
			for key, child := range n {
				if text, ok := child.(string); ok && key == "pattern" {
					found[path] = text
				}
				walk(path+"/"+key, child)
			}
		case []any:
			for i, child := range n {
				walk(fmt.Sprintf("%s/%d", path, i), child)
			}
		}
	}
	walk("", doc)
	return found
}

// sharedEscapes are the escapes Go's regexp (RE2) and JavaScript's RegExp read as the
// same thing. \s and \S are left out: JavaScript's hold every Unicode space, Go's only
// the ASCII ones. \x{..}, \p{..}, \A, \z and \Q are Go's alone; \u and \1 are
// JavaScript's.
const sharedEscapes = `tnrfvdDwW` + `\.^$*+?()[]{}|/-`

// outsideTheSharedSubset says why Go and JavaScript could read pattern differently, or
// "" when they cannot: it keeps to characters, ranges and classes of them, the shared
// escapes, anchors, capturing groups, alternation and repeats.
func outsideTheSharedSubset(pattern string) string {
	for i := 0; i < len(pattern); i++ {
		switch {
		case pattern[i] == '\\':
			i++
			if i == len(pattern) || !strings.ContainsRune(sharedEscapes, rune(pattern[i])) {
				return "an escape the two read differently; write the characters themselves"
			}
		case strings.HasPrefix(pattern[i:], "(?"):
			return "a group syntax only one of them has"
		case strings.HasPrefix(pattern[i:], "[["):
			return "a class syntax only one of them has"
		}
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return err.Error()
	}
	return ""
}

// A pattern in the contract is read by Go and by JavaScript, and must mean the same
// thing to both. One that is more than the subset is a defect in the contract, even
// when Go compiles it.
func TestEveryPatternInTheContractMeansTheSameInGoAndJavaScript(t *testing.T) {
	found := contractPatterns(t)
	if len(found) == 0 {
		t.Fatal("no pattern in the contract: the walk is broken")
	}

	for path, pattern := range found {
		if why := outsideTheSharedSubset(pattern); why != "" {
			t.Errorf("%s: %q: %s", path, pattern, why)
		}
	}
}

// The check must fail on what it is there to catch, and pass what the contract needs.
func TestThePatternCheckSeesWhatItIsThereToCatch(t *testing.T) {
	for _, bad := range []string{`\x{85}`, `\p{Z}`, `\P{L}`, `\pL`, `(?i)a`, `(?:a)`, `\Aa`, `a\z`, `a\Z`, `\Qa\E`, `[[:alpha:]]`, `\u0085`, `\1`, `\s`, `\S`, `\b`, `a\`, `(`} {
		if outsideTheSharedSubset(bad) == "" {
			t.Errorf("%q passes, want it caught", bad)
		}
	}
	for _, good := range []string{`^[A-Za-z0-9-]+$`, "[^\\t-\\r \u0085\u2028]", `\d+\.\d{2}`, `[^\t\n\f\r ]`, `a|b`, `(ab)*c?`} {
		if why := outsideTheSharedSubset(good); why != "" {
			t.Errorf("%q: %s, want it to pass", good, why)
		}
	}
}
