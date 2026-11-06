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
		"log-in":           "/login",
		"log-out":          "/logout",
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
	public := []string{"get-health", "log-in", "read-setup-link", "set-password"}

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
	for _, id := range []string{"log-out", "get-current-user"} {
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
		"log-in":           {"401"},
		"log-out":          {"401"},
		"get-current-user": {"401"},
		"read-setup-link":  {"410"},
		"set-password":     {"410", "422"},
	} {
		for _, status := range statuses {
			if ops[id].Responses[status] == nil {
				t.Errorf("%s does not declare %s; it has %v", id, status, slices.Sorted(maps.Keys(ops[id].Responses)))
			}
		}
	}
}

// ADR 0019: the contract is the list of codes stock-flow-web checks against.
func TestTheContractPublishesEveryErrorCode(t *testing.T) {
	_, contract := api.New(api.Deps{})

	model := contract.OpenAPI().Components.Schemas.Map()["ErrorModel"]

	typ := model.Properties["type"]
	want := []any{"about:blank", "login-failed", "session-ended", "setup-link-invalid"}
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
