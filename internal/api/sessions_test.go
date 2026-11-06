package api_test

import (
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

func TestLogInAnswersTheUserAndSetsTheSessionCookie(t *testing.T) {
	h := newHarness(t)
	h.addUser("somchai@example.com")

	rec := h.logIn("somchai@example.com", password)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	body := decode[map[string]any](t, rec)
	delete(body, "$schema")
	want := map[string]any{"name": "Somchai", "email": "somchai@example.com", "role": "staff"}
	if !maps.Equal(body, want) {
		t.Errorf("body = %v, want %v", body, want)
	}
	// ADR 0015.
	cookie := sessionCookie(t, rec)
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie = %+v, want HttpOnly, Secure and SameSite=Lax", cookie)
	}
	if cookie.Path != "/" {
		t.Errorf("cookie path = %q, want /", cookie.Path)
	}
	if cookie.Value == "" {
		t.Error("cookie has no value")
	}
}

// users-and-access R3.
func TestLogInMatchesTheEmailInAnyLetterCase(t *testing.T) {
	h := newHarness(t)
	h.addUser("Somchai@Example.com")

	for _, email := range []string{"somchai@example.com", "SOMCHAI@EXAMPLE.COM", "Somchai@Example.com"} {
		if rec := h.logIn(email, password); rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200; body %s", email, rec.Code, rec.Body)
		}
	}
}

// failedLogIns seeds one User per cause of a failed login (users-and-access R5).
func failedLogIns(h *harness) map[string][2]string {
	h.addUser("somchai@example.com")
	h.addUser("inactive@example.com", func(u *store.User) { u.Status = "inactive" })
	h.addUser("nopassword@example.com", func(u *store.User) { u.PasswordHash = pgtype.Text{} })
	return map[string][2]string{
		"unknown email":  {"nobody@example.com", password},
		"wrong password": {"somchai@example.com", "wrong password"},
		// The right password, so only the status can fail it.
		"inactive user":   {"inactive@example.com", password},
		"no password yet": {"nopassword@example.com", password},
		"not an email":    {"not an email", "x"},
	}
}

// users-and-access R5: nobody can learn which emails have a User from the answer.
func TestEveryFailedLogInAnswersIdentically(t *testing.T) {
	h := newHarness(t)
	var first string

	for name, attempt := range failedLogIns(h) {
		rec := h.logIn(attempt[0], attempt[1])

		assertProblem(t, rec, http.StatusUnauthorized, "login-failed")
		if got := rec.Header().Get("Set-Cookie"); got != "" {
			t.Errorf("%s: Set-Cookie = %q, want none", name, got)
		}
		if first == "" {
			first = rec.Body.String()
		}
		if rec.Body.String() != first {
			t.Errorf("%s: body = %s, want %s", name, rec.Body, first)
		}
	}
}

// users-and-access R5: nor from the time it took.
func TestEveryFailedLogInTakesTheSameTime(t *testing.T) {
	h := newHarness(t)
	attempts := failedLogIns(h)
	took := map[string]time.Duration{}

	for name, attempt := range attempts {
		start := h.clock.Now()
		h.logIn(attempt[0], attempt[1])
		took[name] = h.clock.Now().Sub(start)
	}

	want := took["wrong password"]
	if want == 0 {
		t.Fatal("a wrong password took no time: no password was verified")
	}
	for name, got := range took {
		if got != want {
			t.Errorf("%s took %v, a wrong password %v", name, got, want)
		}
	}
}

func TestLogInAnswersAStoreFailureWithoutItsCause(t *testing.T) {
	h := newHarness(t)
	h.store.Err = errors.New("connection to db.internal:5432 refused")

	rec := h.logIn("somchai@example.com", password)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "db.internal") {
		t.Errorf("body leaks the cause: %s", rec.Body)
	}
}

// users-and-access R6, R7.
func TestTwoLogInsGiveTwoLiveSessionsAndLogOutEndsOnlyTheCallingOne(t *testing.T) {
	h := newHarness(t)
	h.addUser("somchai@example.com")
	desktop := h.session("somchai@example.com")
	phone := h.session("somchai@example.com")
	if desktop.Value == phone.Value {
		t.Fatal("two logins share a session")
	}
	for name, cookie := range map[string]*http.Cookie{"desktop": desktop, "phone": phone} {
		if rec := h.me(cookie); rec.Code != http.StatusOK {
			t.Fatalf("%s before log out: status = %d; body %s", name, rec.Code, rec.Body)
		}
	}

	rec := h.logOut(desktop)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("log out: status = %d, want 204; body %s", rec.Code, rec.Body)
	}
	assertProblem(t, h.me(desktop), http.StatusUnauthorized, "session-ended")
	if rec := h.me(phone); rec.Code != http.StatusOK {
		t.Errorf("phone after log out: status = %d, want 200; body %s", rec.Code, rec.Body)
	}
}

// Logging out is not a session ending on its own: the next request on this device must
// be a plain visitor's, with no "your session ended" message (users-and-access R26).
func TestLogOutRemovesTheCookie(t *testing.T) {
	h := newHarness(t)
	h.addUser("somchai@example.com")

	rec := h.logOut(h.session("somchai@example.com"))

	cookie := sessionCookie(t, rec)
	if cookie.MaxAge >= 0 || cookie.Value != "" {
		t.Errorf("cookie = %+v, want it removed", cookie)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/" {
		t.Errorf("cookie = %+v, want the attributes it was set with", cookie)
	}
}

// users-and-access R21.
func TestASessionEndsSevenDaysAfterLogInHoweverMuchItIsUsed(t *testing.T) {
	h := newHarness(t)
	h.addUser("somchai@example.com")
	cookie := h.session("somchai@example.com")

	for range 6 {
		h.clock.Advance(24 * time.Hour)
		if rec := h.me(cookie); rec.Code != http.StatusOK {
			t.Fatalf("at %v: status = %d, want 200; body %s", h.clock.Now(), rec.Code, rec.Body)
		}
	}
	h.clock.Advance(24*time.Hour - time.Second)
	if rec := h.me(cookie); rec.Code != http.StatusOK {
		t.Fatalf("a second before 7 days: status = %d, want 200; body %s", rec.Code, rec.Body)
	}

	h.clock.Advance(time.Second)

	assertProblem(t, h.me(cookie), http.StatusUnauthorized, "session-ended")
}

// users-and-access R26: if the cookie died with its session, a session ended by time
// would look like a first visit.
func TestTheCookieOutlivesItsSession(t *testing.T) {
	h := newHarness(t)
	h.addUser("somchai@example.com")

	cookie := sessionCookie(t, h.logIn("somchai@example.com", password))

	if sevenDays := int((7 * 24 * time.Hour).Seconds()); cookie.MaxAge <= sevenDays {
		t.Errorf("Max-Age = %d, want more than %d", cookie.MaxAge, sevenDays)
	}
}

// users-and-access R26. Every operation that needs a session answers the same way.
func TestARequestWithNoLiveSessionIsRefused(t *testing.T) {
	operations := map[string]func(*harness, ...*http.Cookie) *httptest.ResponseRecorder{
		"get current user": (*harness).me,
		"log out":          (*harness).logOut,
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)

			noCookie := operation(h)
			assertProblem(t, noCookie, http.StatusUnauthorized, "")

			unknown := operation(h, &http.Cookie{Name: "session", Value: "no-such-session"})
			assertProblem(t, unknown, http.StatusUnauthorized, "session-ended")
		})
	}
}

// users-and-access R8, R14: every Role, and a name in any language.
func TestGetCurrentUserAnswersNameEmailAndRole(t *testing.T) {
	for _, role := range []string{"owner", "admin", "staff"} {
		t.Run(role, func(t *testing.T) {
			h := newHarness(t)
			h.addUser("somchai@example.com", func(u *store.User) { u.Name, u.Role = "สมชาย", role })

			rec := h.me(h.session("somchai@example.com"))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
			}
			body := decode[map[string]any](t, rec)
			delete(body, "$schema")
			want := map[string]any{"name": "สมชาย", "email": "somchai@example.com", "role": role}
			if !maps.Equal(body, want) {
				t.Errorf("body = %v, want %v", body, want)
			}
		})
	}
}

func TestASessionTokenIsNotStoredAsIs(t *testing.T) {
	h := newHarness(t)
	h.addUser("somchai@example.com")
	cookie := h.session("somchai@example.com")

	_, err := h.store.GetSession(t.Context(), []byte(cookie.Value))

	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the cookie's value is the stored key: err = %v", err)
	}
}
