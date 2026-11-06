package api_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/api"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store/storetest"
)

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHealthReportsOKWhenTheDatabaseAnswers(t *testing.T) {
	h, _ := api.New(api.Deps{Store: storetest.NewFake()})

	rec := get(t, h, "/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body, err)
	}
	if body.Status != "ok" {
		t.Errorf("status field = %q, want %q", body.Status, "ok")
	}
}

func TestHealthReportsUnavailableAsProblemDetailsWhenTheDatabaseFails(t *testing.T) {
	h, _ := api.New(api.Deps{Store: &storetest.Fake{Err: errors.New("connection refused")}})

	rec := get(t, h, "/health")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	var problem struct {
		Status int    `json:"status"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body, err)
	}
	if problem.Status != http.StatusServiceUnavailable || problem.Detail != "database unreachable" {
		t.Errorf("problem = %+v, want status 503 and detail %q", problem, "database unreachable")
	}
}

// Vercel caches external rewrites by the upstream Cache-Control, so every response,
// including errors and unknown paths, must forbid caching.
func TestEveryResponseForbidsCaching(t *testing.T) {
	cases := map[string]struct {
		store api.Store
		path  string
	}{
		"health ok":          {storetest.NewFake(), "/health"},
		"health unavailable": {&storetest.Fake{Err: errors.New("down")}, "/health"},
		"unknown path":       {storetest.NewFake(), "/no-such-path"},
		"published contract": {storetest.NewFake(), "/openapi.yaml"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h, _ := api.New(api.Deps{Store: tc.store})

			rec := get(t, h, tc.path)

			if got := rec.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store (status %d)", got, rec.Code)
			}
		})
	}
}
