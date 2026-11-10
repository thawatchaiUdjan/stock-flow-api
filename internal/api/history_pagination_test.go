package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/api"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// context ADR 0009: explicit offset selects numbered mode; #36 stages it as 501.
func TestHistoryNumberedModeIsTemporarilyUnavailableOnlyWithExplicitOffset(t *testing.T) {
	for _, path := range []string{"/stock-movements", "/login-log"} {
		t.Run(path, func(t *testing.T) {
			h := newHarness(t)
			s := h.shop()
			for _, query := range []string{"offset=0", "offset=25&limit=25", "offset=2147483647&limit=100"} {
				rec := h.do(http.MethodGet, path+"?"+query, nil, s.owner)
				assertProblem(t, rec, http.StatusNotImplemented, "")
				if got := decode[problem](t, rec).Detail; got != "Numbered history pagination is not implemented yet." {
					t.Errorf("detail = %q", got)
				}
			}
		})
	}
}

func TestHistoryCursorResponsesOmitTotalAndPreserveEmptyPages(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	for path, field := range map[string]string{"/stock-movements": "movements", "/login-log": "entries"} {
		for _, query := range []string{"", "?before=1", "?limit=25&before=1"} {
			rec := h.do(http.MethodGet, path+query, nil, s.owner)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s%s: %d: %s", path, query, rec.Code, rec.Body)
			}
			body := decode[map[string]json.RawMessage](t, rec)
			if _, present := body["total"]; present {
				t.Errorf("%s%s: legacy response contains total: %s", path, query, rec.Body)
			}
			if query != "" || path == "/stock-movements" {
				if string(body[field]) != "[]" || string(body["nextBefore"]) != "null" {
					t.Errorf("%s%s: want empty page with null cursor: %s", path, query, rec.Body)
				}
			}
		}
	}
}

// users-and-access R4, R10, R23: numbered mode retains the existing access gate.
func TestHistoryNumberedModePreservesAuthorizationBeforeValidation(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	for _, path := range []string{"/stock-movements", "/login-log"} {
		for _, query := range []string{"offset=0", "offset=-1", "offset=0&before=1"} {
			assertProblem(t, h.do(http.MethodGet, path+"?"+query, nil), http.StatusUnauthorized, "")
			for _, cookie := range []*http.Cookie{s.admin, s.staff} {
				rec := h.do(http.MethodGet, path+"?"+query, nil, cookie)
				if path == "/login-log" {
					assertProblem(t, rec, http.StatusForbidden, "")
				} else if query == "offset=0" {
					assertProblem(t, rec, http.StatusNotImplemented, "")
				} else {
					assertProblem(t, rec, http.StatusUnprocessableEntity, "")
				}
			}
		}
	}
}

type unavailableHistoryStore struct{ api.Store }

func (s unavailableHistoryStore) ListLoginLog(context.Context, store.ListLoginLogParams) ([]store.ListLoginLogRow, error) {
	return nil, errors.New("private database failure")
}

func (s unavailableHistoryStore) ListStockMovements(context.Context, store.ListStockMovementsParams) ([]store.ListStockMovementsRow, error) {
	return nil, errors.New("private database failure")
}

func TestHistoryCursorFailuresStayPrivateAndNumberedModeDoesNotReadHistory(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	h.handler, _ = api.New(api.Deps{Store: unavailableHistoryStore{h.store}, Now: h.clock.Now})
	for _, path := range []string{"/stock-movements", "/login-log"} {
		rec := h.do(http.MethodGet, path, nil, s.owner)
		assertProblem(t, rec, http.StatusInternalServerError, "")
		if strings.Contains(rec.Body.String(), "private database failure") {
			t.Errorf("%s: internal error disclosed: %s", path, rec.Body)
		}
		assertProblem(t, h.do(http.MethodGet, path+"?offset=0", nil, s.owner), http.StatusNotImplemented, "")
	}
}

// navigation-and-lists R6, context ADR 0009: publish the numbered success contract
// even while its implementation is staged behind the temporary 501 response.
func TestHistoryContractPublishesNumberedModeAndConditionalExactTotal(t *testing.T) {
	h := newHarness(t)
	for _, path := range []string{"/stock-movements", "/login-log"} {
		op := h.contract.OpenAPI().Paths[path].Get
		for _, status := range []string{"200", "401", "422", "500", "501"} {
			if op.Responses[status] == nil {
				t.Errorf("%s: missing %s response", path, status)
			}
		}
		params := map[string]*huma.Param{}
		for _, p := range op.Parameters {
			params[p.Name] = p
		}
		offset := params["offset"]
		if offset == nil || offset.Required || offset.Schema.Type != "integer" || offset.Schema.Minimum == nil || *offset.Schema.Minimum != 0 || offset.Schema.Maximum == nil || *offset.Schema.Maximum != 2147483647 || offset.Schema.Default != nil {
			t.Errorf("%s: offset must be optional bounded integer without a default: %+v", path, offset)
		}
		limit := params["limit"].Schema
		if *limit.Minimum != 1 || *limit.Maximum != 100 || limit.Default != 50 {
			t.Errorf("%s: legacy limit changed: %+v", path, limit)
		}
		response := op.Responses["200"].Content["application/json"].Schema
		body := h.contract.OpenAPI().Components.Schemas.Map()[strings.TrimPrefix(response.Ref, "#/components/schemas/")]
		total := body.Properties["total"]
		if total == nil || total.Type != "integer" || total.Minimum == nil || *total.Minimum != 0 || slices.Contains(body.Required, "total") {
			t.Errorf("%s: total must be optional nonnegative integer: %+v", path, total)
		}
		for _, phrase := range []string{"always present", "all current filters", "total: 0", "501", "422", "offset=0"} {
			if !strings.Contains(op.Description, phrase) {
				t.Errorf("%s: description omits %q", path, phrase)
			}
		}
	}
}

// navigation-and-lists R4-R6: refuse invalid paging before reaching the store.
func TestHistoryNumberedModeValidatesBoundsAndRejectsCursorMixing(t *testing.T) {
	for _, path := range []string{"/stock-movements", "/login-log"} {
		t.Run(path, func(t *testing.T) {
			h := newHarness(t)
			s := h.shop()
			for _, tc := range []struct{ query, field string }{
				{"offset=-1", "offset"},
				{"offset=2147483648", "offset"},
				{"offset=999999999999999999999999999999", "offset"},
				{"offset=1.5", "offset"},
				{"offset=abc", "offset"},
				{"offset=", "offset"},
				{"offset=0&before=1", "before"},
				{"offset=25&before=1", "before"},
				{"offset=0&before=", "before"},
				{"offset=0&limit=0", "limit"},
				{"offset=0&limit=101", "limit"},
			} {
				t.Run(tc.query, func(t *testing.T) {
					rec := h.do(http.MethodGet, path+"?"+tc.query, nil, s.owner)
					assertFieldError(t, rec, "query."+tc.field)
				})
			}
		})
	}
}
