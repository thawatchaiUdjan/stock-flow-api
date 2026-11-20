package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

var limitsBase = time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)

// loginAt logs in from ip with the clock at t. Each attempt is read at t: the fake hasher
// moves the clock afterwards.
func (h *harness) loginAt(t time.Time, ip, email, pw string) *httptest.ResponseRecorder {
	h.t.Helper()
	h.clock.Set(t)
	body, _ := json.Marshal(map[string]string{"email": email, "password": pw})
	return h.from(ip+":1234", nil, http.MethodPost, "/login", string(body))
}

func (h *harness) failAt(t time.Time, ip, email string) {
	h.t.Helper()
	assertProblem(h.t, h.loginAt(t, ip, email, "wrong password"), http.StatusUnauthorized, "login-failed")
}

func assertBlocked(t *testing.T, rec *httptest.ResponseRecorder, retryAfter time.Duration) {
	t.Helper()
	assertProblem(t, rec, http.StatusTooManyRequests, "login-blocked")
	if got := rec.Header().Get("Retry-After"); got != strconv.Itoa(int(retryAfter.Seconds())) {
		t.Errorf("Retry-After = %q, want %d seconds", got, int(retryAfter.Seconds()))
	}
}

// users-and-access R55: 5 failures within 15 minutes block the email until 15 minutes
// after the last failure, even for the right password.
func TestFiveFailedLoginsBlockTheEmailUntilFifteenMinutesAfterTheLastFailure(t *testing.T) {
	h := newHarness(t)
	h.addUser("somchai@example.com")
	ip := "203.0.113.7"
	for i := range 4 {
		h.failAt(limitsBase.Add(time.Duration(i)*time.Minute), ip, "somchai@example.com")
	}
	// Four failures block nothing; a success does not clear them (it is not a failure).
	if rec := h.loginAt(limitsBase.Add(4*time.Minute), ip, "somchai@example.com", password); rec.Code != http.StatusOK {
		t.Fatalf("after 4 failures: status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	h.failAt(limitsBase.Add(4*time.Minute), ip, "somchai@example.com") // the 5th; the last failure is at +4m

	// The oldest failure left the 15 minutes at +15m, but the block runs to +19m.
	assertBlocked(t, h.loginAt(limitsBase.Add(16*time.Minute), ip, "somchai@example.com", password), 3*time.Minute)
	// A blocked attempt is not a failure: it does not extend the block.
	assertBlocked(t, h.loginAt(limitsBase.Add(19*time.Minute-time.Second), ip, "somchai@example.com", password), time.Second)
	if rec := h.loginAt(limitsBase.Add(19*time.Minute), ip, "somchai@example.com", password); rec.Code != http.StatusOK {
		t.Fatalf("15 minutes after the last failure: status = %d, want 200; body %s", rec.Code, rec.Body)
	}
}

// users-and-access R55: failures further apart than 15 minutes never add up.
func TestFiveFailuresSpreadOverMoreThanFifteenMinutesBlockNothing(t *testing.T) {
	h := newHarness(t)
	h.addUser("somchai@example.com")
	for i := range 5 {
		h.failAt(limitsBase.Add(time.Duration(i)*4*time.Minute), "203.0.113.7", "somchai@example.com") // +0 to +16m
	}
	if rec := h.loginAt(limitsBase.Add(16*time.Minute), "203.0.113.7", "somchai@example.com", password); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
}

// users-and-access R55: any email counts, and R57: the refusal tells nothing about it.
func TestABlockedLoginSaysTheSameForKnownUnknownAndInactiveEmails(t *testing.T) {
	h := newHarness(t)
	h.addUser("known@example.com")
	h.addUser("inactive@example.com", func(u *store.User) { u.Status = "inactive" })
	emails := []string{"known@example.com", "inactive@example.com", "nobody@example.com"}
	for i, email := range emails {
		for j := range 5 {
			h.failAt(limitsBase.Add(time.Duration(j)*time.Second), fmt.Sprintf("203.0.113.%d", i+1), email)
		}
	}
	var first string
	for _, email := range emails {
		rec := h.loginAt(limitsBase.Add(10*time.Second), "198.51.100.1", email, password)
		assertProblem(t, rec, http.StatusTooManyRequests, "login-blocked")
		if first == "" {
			first = rec.Body.String()
		}
		if rec.Body.String() != first {
			t.Errorf("%s: body = %s, want %s", email, rec.Body, first)
		}
	}
	// The block is per email, in any letter case.
	assertProblem(t, h.loginAt(limitsBase.Add(10*time.Second), "198.51.100.1", "KNOWN@example.com", password), http.StatusTooManyRequests, "login-blocked")
	// Another email from the same address is not blocked.
	h.addUser("other@example.com")
	if rec := h.loginAt(limitsBase.Add(10*time.Second), "198.51.100.1", "other@example.com", password); rec.Code != http.StatusOK {
		t.Errorf("other email: status = %d, want 200; body %s", rec.Code, rec.Body)
	}
}

// users-and-access R56: 50 failures from one address block it until fewer than 50 remain
// in the last hour.
func TestFiftyFailedLoginsBlockTheAddressUntilFewerThanFiftyRemainInTheHour(t *testing.T) {
	h := newHarness(t)
	h.addUser("somchai@example.com")
	ip := "203.0.113.7"
	for i := range 49 {
		h.failAt(limitsBase.Add(time.Duration(i)*time.Minute), ip, fmt.Sprintf("guess%d@example.com", i))
	}
	if rec := h.loginAt(limitsBase.Add(49*time.Minute), ip, "somchai@example.com", password); rec.Code != http.StatusOK {
		t.Fatalf("after 49 failures: status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	h.failAt(limitsBase.Add(49*time.Minute), ip, "guess49@example.com")

	assertBlocked(t, h.loginAt(limitsBase.Add(50*time.Minute), ip, "somchai@example.com", password), 10*time.Minute)
	// Another address is not blocked.
	if rec := h.loginAt(limitsBase.Add(50*time.Minute), "203.0.113.8", "somchai@example.com", password); rec.Code != http.StatusOK {
		t.Errorf("other address: status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	// The oldest failure (+0m) leaves the hour at +1h: 49 remain.
	assertBlocked(t, h.loginAt(limitsBase.Add(time.Hour-time.Second), ip, "somchai@example.com", password), time.Second)
	if rec := h.loginAt(limitsBase.Add(time.Hour), ip, "somchai@example.com", password); rec.Code != http.StatusOK {
		t.Fatalf("an hour after the oldest: status = %d, want 200; body %s", rec.Code, rec.Body)
	}
}

// users-and-access R36, R57: each blocked attempt is in the Login log as blocked.
func TestEachBlockedLoginIsInTheLoginLog(t *testing.T) {
	h := newHarness(t)
	user := h.addUser("somchai@example.com")
	owner := h.addUser("owner@example.com", func(u *store.User) { u.Role = "owner" })
	ownerSession := h.session(owner.Email)
	for i := range 5 {
		h.failAt(limitsBase.Add(time.Duration(i)*time.Second), "203.0.113.7", "somchai@example.com")
	}
	h.clock.Set(limitsBase.Add(10 * time.Second))
	h.from("203.0.113.7:1", http.Header{"User-Agent": {"Firefox"}}, http.MethodPost, "/login", `{"email":"Somchai@Example.com","password":"x"}`)
	h.loginAt(limitsBase.Add(11*time.Second), "203.0.113.7", "nobody@example.com", password) // the address has 5 failures only

	page := h.loginLog("?result=blocked", ownerSession)

	if len(page.Entries) != 1 {
		t.Fatalf("blocked entries = %+v, want 1", page.Entries)
	}
	got := page.Entries[0]
	if got.Email != "Somchai@Example.com" || got.User == nil || got.User.ID != user.ID || got.IP != "203.0.113.7" || got.UserAgent != "Firefox" || got.Result != "blocked" {
		t.Errorf("entry = %+v", got)
	}
}

// users-and-access R57: Forgot password and password setup are never blocked.
func TestBlockedLoginsDoNotBlockForgotPasswordOrPasswordSetup(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addUser("somchai@example.com")
	for i := range 5 {
		h.failAt(limitsBase.Add(time.Duration(i)*time.Second), "203.0.113.7", "somchai@example.com")
	}
	assertProblem(t, h.loginAt(limitsBase.Add(10*time.Second), "203.0.113.7", "somchai@example.com", password), http.StatusTooManyRequests, "login-blocked")

	for _, path := range []string{"/v2/password-reset/request", "/v2/password-setup/request"} {
		rec := h.from("203.0.113.7:1", nil, http.MethodPost, path, `{"email":"somchai@example.com"}`)
		if rec.Code != http.StatusAccepted {
			t.Errorf("%s: status = %d, want 202; body %s", path, rec.Code, rec.Body)
		}
	}
	if len(m.messages) == 0 {
		t.Error("Forgot password sent nothing")
	}
}

// users-and-access R19, business ADR 0017: 6 to 128 characters, on every operation that
// sets a password.
func TestEveryPasswordSettingOperationRefusesMoreThan128Characters(t *testing.T) {
	longest := strings.Repeat("ก", 128)
	tooLong := strings.Repeat("ก", 129)
	setters := map[string]func(t *testing.T) func(pw string) *httptest.ResponseRecorder{
		"set-password": func(t *testing.T) func(string) *httptest.ResponseRecorder {
			h := newHarness(t)
			token := h.installOwner()
			return func(pw string) *httptest.ResponseRecorder { return h.setPassword(token, pw) }
		},
		"set-email-password": func(t *testing.T) func(string) *httptest.ResponseRecorder {
			h, m := newEmailHarness(t)
			h.addUser("new@example.com", func(u *store.User) { u.PasswordHash = pgtype.Text{} })
			h.requestCode("new@example.com")
			grant := decode[verificationBody](t, h.verifyCode("new@example.com", m.messages[0].Code))
			return func(pw string) *httptest.ResponseRecorder {
				return h.do(http.MethodPost, "/v2/password-setup/password", map[string]string{"token": grant.Token, "password": pw})
			}
		},
		"reset-email-password": func(t *testing.T) func(string) *httptest.ResponseRecorder {
			h, m := newEmailHarness(t)
			h.addUser("user@example.com")
			h.requestReset("user@example.com")
			token := resetToken(t, m.messages[0])
			return func(pw string) *httptest.ResponseRecorder { return h.saveReset(token, pw) }
		},
	}
	for name, setup := range setters {
		t.Run(name, func(t *testing.T) {
			set := setup(t)
			rec := set(tooLong)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("129 characters: status = %d, want 422; body %s", rec.Code, rec.Body)
			}
			if got := decode[problem](t, rec); len(got.Errors) != 1 || got.Errors[0].Location != "body.password" {
				t.Errorf("errors = %+v, want one at body.password", got.Errors)
			}
			if rec := set(longest); rec.Code != http.StatusNoContent {
				t.Errorf("128 characters (the refusal kept the link): status = %d, want 204; body %s", rec.Code, rec.Body)
			}
		})
	}
}

// users-and-access R19: an existing password shorter than the minimum still logs in, and
// a login does not apply the length rules (R5: a wrong guess fails like any other).
func TestLoggingInStillAcceptsAnyPasswordLength(t *testing.T) {
	h := newHarness(t)
	h.addUser("somchai@example.com", func(u *store.User) { u.PasswordHash = pgtype.Text{String: cheap.Hash("abc"), Valid: true} })

	if rec := h.logIn("somchai@example.com", "abc"); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	assertProblem(t, h.logIn("somchai@example.com", strings.Repeat("x", 200)), http.StatusUnauthorized, "login-failed")
}

// ADR 0019: the code is published and declared on the login operation.
func TestLoginBlockedIsPublishedAndDeclaredOnLogin(t *testing.T) {
	h := newHarness(t)
	op := operations(t)["create-session"]
	if op.Responses["429"] == nil {
		t.Error("create-session does not declare 429")
	}
	if !strings.Contains(op.Description, "`login-blocked`") {
		t.Errorf("description %q does not name login-blocked", op.Description)
	}
	enum := h.contract.OpenAPI().Components.Schemas.Map()["ErrorModel"].Properties["type"].Enum
	found := false
	for _, v := range enum {
		found = found || v == "login-blocked"
	}
	if !found {
		t.Errorf("type enum = %v, want login-blocked in it", enum)
	}
}
