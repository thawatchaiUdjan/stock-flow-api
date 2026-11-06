package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/api"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store/storetest"
)

// cheap is the cheapest Argon2id: tests hash a lot (docs/agents/testing.md).
var cheap = auth.PasswordHasher{Memory: 8, Time: 1}

const password = "correct horse"

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// timedHasher moves the clock on each verification the way a real hash takes time, so
// a fast test can compare how long two logins took.
type timedHasher struct {
	auth.PasswordHasher
	clock *fakeClock
}

func (p timedHasher) Verify(encoded, password string) bool {
	p.clock.Advance(100 * time.Millisecond)
	return p.PasswordHasher.Verify(encoded, password)
}

// harness is the whole HTTP handler over a fake store and a fake clock.
type harness struct {
	t        *testing.T
	handler  http.Handler
	contract huma.API
	store    *storetest.Fake
	clock    *fakeClock
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	fake := storetest.NewFake()
	clock := &fakeClock{now: time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)}
	handler, contract := api.New(api.Deps{
		Store:          fake,
		PasswordHasher: timedHasher{PasswordHasher: cheap, clock: clock},
		Now:            clock.Now,
	})
	return &harness{t: t, handler: handler, contract: contract, store: fake, clock: clock}
}

// addUser seeds an Active Staff User with a password, unless change says otherwise.
func (h *harness) addUser(email string, change ...func(*store.User)) store.User {
	h.t.Helper()
	user := store.User{
		Name: "Somchai", Email: email, Role: "staff", Status: "active",
		PasswordHash: pgtype.Text{String: cheap.Hash(password), Valid: true},
	}
	for _, c := range change {
		c(&user)
	}
	return h.store.AddUser(user)
}

func (h *harness) do(method, path string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			h.t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &payload)
	req.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		req.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

func (h *harness) logIn(email, password string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, "/login", map[string]string{"email": email, "password": password})
}

// session logs in and returns the session cookie the API set.
func (h *harness) session(email string) *http.Cookie {
	h.t.Helper()
	rec := h.logIn(email, password)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("log in: status = %d; body %s", rec.Code, rec.Body)
	}
	return sessionCookie(h.t, rec)
}

func (h *harness) me(cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodGet, "/me", nil, cookies...)
}

func (h *harness) logOut(cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, "/logout", nil, cookies...)
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "session" {
			return cookie
		}
	}
	t.Fatalf("no session cookie in %v", rec.Header())
	return nil
}

type problem struct {
	Type   string `json:"type"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
	Errors []struct {
		Location string `json:"location"`
		Message  string `json:"message"`
	} `json:"errors"`
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(rec.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body, err)
	}
	return value
}

func assertProblem(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body %s", rec.Code, status, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	if got := decode[problem](t, rec); got.Type != code || got.Status != status {
		t.Errorf("problem = %+v, want type %q and status %d", got, code, status)
	}
}
