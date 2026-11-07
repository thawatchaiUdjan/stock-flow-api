package api_test

import (
	"maps"
	"net/http"
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
	public := []string{"create-session", "get-health", "read-setup-link", "set-password"}

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
	for _, id := range []string{"end-session", "get-current-user", "list-users", "create-user", "update-user", "reset-password"} {
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

	for _, id := range []string{"read-setup-link", "set-password"} {
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
		"create-session":   {"401"},
		"end-session":      {"401"},
		"get-current-user": {"401"},
		"read-setup-link":  {"410"},
		"set-password":     {"410", "422"},
		"list-users":       {"401", "403"},
		"create-user":      {"401", "403", "409", "422"},
		"update-user":      {"401", "403", "404", "422"},
		"reset-password":   {"401", "403", "404", "409"},
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
		"create-session":   "login-failed",
		"end-session":      "session-ended",
		"get-current-user": "session-ended",
		"read-setup-link":  "setup-link-invalid",
		"set-password":     "setup-link-invalid",
		"list-users":       "session-ended",
		"create-user":      "email-taken",
		"update-user":      "target-is-owner",
		"reset-password":   "user-inactive",
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
	want := []any{"about:blank", "login-failed", "session-ended", "setup-link-invalid", "email-taken", "target-is-owner", "user-inactive", "sku-taken"}
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
