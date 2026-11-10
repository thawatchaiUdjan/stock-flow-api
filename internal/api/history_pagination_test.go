package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/api"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// context ADR 0009: explicit zero selects numbered mode and zero is a real total.
func TestHistoryNumberedModeReturnsExactTotalsEvenBeyondTheLastPage(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	for _, path := range []string{"/stock-movements", "/login-log?result=logout"} {
		for _, offset := range []string{"0", "25", "2147483647"} {
			separator := "?"
			if strings.Contains(path, "?") {
				separator = "&"
			}
			rec := h.do(http.MethodGet, path+separator+"offset="+offset+"&limit=25", nil, s.owner)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: %d: %s", path, rec.Code, rec.Body)
			}
			body := decode[map[string]json.RawMessage](t, rec)
			if string(body["total"]) != "0" || string(body["nextBefore"]) != "null" {
				t.Errorf("%s: %s", path, rec.Body)
			}
		}
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
					if rec.Code != http.StatusOK {
						t.Fatalf("numbered stock movements: %d: %s", rec.Code, rec.Body)
					}
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

func TestHistoryPageFailuresStayPrivateInBothModes(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	h.handler, _ = api.New(api.Deps{Store: unavailableHistoryStore{h.store}, Now: h.clock.Now})
	for _, path := range []string{"/stock-movements", "/login-log"} {
		rec := h.do(http.MethodGet, path, nil, s.owner)
		assertProblem(t, rec, http.StatusInternalServerError, "")
		if strings.Contains(rec.Body.String(), "private database failure") {
			t.Errorf("%s: internal error disclosed: %s", path, rec.Body)
		}
		rec = h.do(http.MethodGet, path+"?offset=0", nil, s.owner)
		assertProblem(t, rec, http.StatusInternalServerError, "")
		if strings.Contains(rec.Body.String(), "private database failure") {
			t.Errorf("internal error disclosed: %s", rec.Body)
		}
	}
}

// navigation-and-lists R6, context ADR 0009: publish the numbered success contract
// with totals conditional on the explicitly selected mode.
func TestHistoryContractPublishesNumberedModeAndConditionalExactTotal(t *testing.T) {
	h := newHarness(t)
	for _, path := range []string{"/stock-movements", "/login-log"} {
		op := h.contract.OpenAPI().Paths[path].Get
		for _, status := range []string{"200", "401", "422", "500"} {
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
		for _, phrase := range []string{"always present", "all current filters", "total: 0", "422", "offset=0"} {
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

// navigation-and-lists R4-R6, context ADR 0009: the same bounded pages work for all movement roles.
func TestHistoryNumberedPagesIncludeMiddleLastAndOutOfRangeWithFilteredTotals(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	tea := h.seed(store.Product{Name: "Tea", Sku: "TEA"})
	var made []int64
	for range 5 {
		made = append(made, decode[movementBody](t, h.receive(map[string]any{"productId": tea.ID, "quantity": 1}, s.staff)).ID)
		assertProblem(t, h.do(http.MethodPost, "/login", map[string]any{"email": "nobody@example.com", "password": "wrong"}), http.StatusUnauthorized, "login-failed")
	}
	for _, path := range []string{"/stock-movements?kind=receive", "/login-log?result=failed-login"} {
		for _, cookie := range []*http.Cookie{s.owner, s.admin, s.staff} {
			if strings.HasPrefix(path, "/login-log") && cookie != s.owner {
				continue
			}
			var collected []int64
			for _, tc := range []struct{ offset, size int }{{0, 2}, {2, 2}, {4, 1}, {5, 0}, {2147483647, 0}} {
				rec := h.do(http.MethodGet, fmt.Sprintf("%s&offset=%d&limit=2", path, tc.offset), nil, cookie)
				if rec.Code != http.StatusOK {
					t.Fatalf("%s: %d: %s", path, rec.Code, rec.Body)
				}
				body := decode[struct {
					Movements  []struct{ ID int64 }
					Entries    []struct{ ID int64 }
					Total      *int64
					NextBefore *int64
				}](t, rec)
				rows := body.Movements
				if strings.HasPrefix(path, "/login-log") {
					rows = body.Entries
				}
				if body.Total == nil || *body.Total != 5 || len(rows) != tc.size || body.NextBefore != nil {
					t.Fatalf("%s offset %d: %s", path, tc.offset, rec.Body)
				}
				for _, row := range rows {
					collected = append(collected, row.ID)
				}
			}
			if strings.HasPrefix(path, "/stock-movements") {
				want := []int64{made[4], made[3], made[2], made[1], made[0]}
				if !slices.Equal(collected, want) {
					t.Errorf("ids = %v, want %v", collected, want)
				}
			} else if len(collected) != 5 || !slices.IsSortedFunc(collected, func(a, b int64) int {
				if a > b {
					return -1
				}
				if a < b {
					return 1
				}
				return 0
			}) {
				t.Errorf("login ids = %v", collected)
			}
		}
	}
}

type unavailableHistoryCountStore struct{ api.Store }

func (s unavailableHistoryCountStore) CountLoginLog(context.Context, store.CountLoginLogParams) (int64, error) {
	return 0, errors.New("private count failure")
}
func (s unavailableHistoryCountStore) CountStockMovements(context.Context, store.CountStockMovementsParams) (int64, error) {
	return 0, errors.New("private count failure")
}

func TestHistoryCountFailuresStayPrivateAndDoNotAffectCursorClients(t *testing.T) {
	h := newHarness(t)
	s := h.shop()
	h.handler, _ = api.New(api.Deps{Store: unavailableHistoryCountStore{h.store}, Now: h.clock.Now})
	for _, path := range []string{"/stock-movements", "/login-log"} {
		rec := h.do(http.MethodGet, path+"?offset=0", nil, s.owner)
		assertProblem(t, rec, http.StatusInternalServerError, "")
		if strings.Contains(rec.Body.String(), "private count failure") {
			t.Errorf("private count error disclosed: %s", rec.Body)
		}
		if rec = h.do(http.MethodGet, path, nil, s.owner); rec.Code != http.StatusOK {
			t.Errorf("cursor calls count: %d: %s", rec.Code, rec.Body)
		}
	}
}
