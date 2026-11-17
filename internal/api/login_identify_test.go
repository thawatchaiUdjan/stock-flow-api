package api_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/email"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

type identifyBody struct {
	Next         string `json:"next"`
	CodeDelivery string `json:"codeDelivery"`
}

func (h *harness) identify(address string) identifyBody {
	h.t.Helper()
	rec := h.do(http.MethodPost, "/v2/login/identify", map[string]string{"email": address})
	if rec.Code != http.StatusOK {
		h.t.Fatalf("identify %s = %d, %s", address, rec.Code, rec.Body)
	}
	return decode[identifyBody](h.t, rec)
}

// users-and-access R50: every Role with a password continues to the password step.
func TestIdentifySendsUsersWithAPasswordToThePasswordStep(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addUser("staff@example.com")
	h.addUser("admin@example.com", func(u *store.User) { u.Role = "admin" })
	h.addUser("owner@example.com", func(u *store.User) { u.Role = "owner" })
	for _, address := range []string{"staff@example.com", "admin@example.com", "owner@example.com"} {
		if got := h.identify(address); got != (identifyBody{Next: "password"}) {
			t.Errorf("%s = %+v, want password without codeDelivery", address, got)
		}
	}
	if len(m.messages) != 0 {
		t.Fatalf("password step sent %d emails", len(m.messages))
	}
}

// users-and-access R50, R16: an Inactive User is indistinguishable from an unknown email.
func TestIdentifyAnswersUnknownAndInactiveEmailsIdentically(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addUser("gone@example.com", func(u *store.User) { u.Status = "inactive" })
	h.addUser("unfinished@example.com", func(u *store.User) { u.Status = "inactive"; u.PasswordHash = pgtype.Text{} })
	unknown := h.do(http.MethodPost, "/v2/login/identify", map[string]string{"email": "nobody@example.com"})
	for _, address := range []string{"gone@example.com", "unfinished@example.com"} {
		inactive := h.do(http.MethodPost, "/v2/login/identify", map[string]string{"email": address})
		if inactive.Code != unknown.Code || inactive.Body.String() != unknown.Body.String() {
			t.Errorf("%s = %d %s; unknown = %d %s", address, inactive.Code, inactive.Body, unknown.Code, unknown.Body)
		}
	}
	if got := decode[identifyBody](t, unknown); got != (identifyBody{Next: "no-account"}) {
		t.Fatalf("unknown = %+v, want no-account", got)
	}
	if len(m.messages) != 0 {
		t.Fatalf("no-account sent %d emails", len(m.messages))
	}
}

// users-and-access R34, R52: before installation setup the Owner is pointed to the
// installation Setup link and sent nothing.
func TestIdentifyPointsTheOwnerWithoutAPasswordToTheInstallationSetupLink(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addUser("owner@example.com", func(u *store.User) { u.Role = "owner"; u.PasswordHash = pgtype.Text{} })
	for range 2 {
		if got := h.identify("OWNER@example.com"); got != (identifyBody{Next: "installation-link"}) {
			t.Fatalf("owner = %+v, want installation-link", got)
		}
	}
	if len(m.messages) != 0 {
		t.Fatalf("installation-link sent %d emails", len(m.messages))
	}
}

// users-and-access R51, R29, R27: the email step sends a First-time code that verifies,
// and a later one replaces it.
func TestIdentifySendsAFirstTimeCodeToAnAdminOrStaffWithoutAPassword(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addUser("staff@example.com", func(u *store.User) { u.PasswordHash = pgtype.Text{} })
	h.addUser("admin@example.com", func(u *store.User) { u.Role = "admin"; u.PasswordHash = pgtype.Text{} })
	for i, address := range []string{"staff@example.com", "ADMIN@example.com"} {
		if got := h.identify(address); got != (identifyBody{Next: "first-time-code", CodeDelivery: "sent"}) {
			t.Fatalf("%s = %+v, want first-time-code sent", address, got)
		}
		if len(m.messages) != i+1 || m.messages[i].Kind != email.FirstTimeCode || !strings.EqualFold(m.messages[i].To, address) {
			t.Fatalf("messages = %+v", m.messages)
		}
	}
	first := m.messages[0].Code
	h.clock.Advance(time.Minute)
	h.identify("staff@example.com")
	latest := m.messages[2].Code
	if first != latest {
		assertProblem(t, h.verifyCode("staff@example.com", first), http.StatusGone, "first-time-code-invalid")
	}
	if rec := h.verifyCode("staff@example.com", latest); rec.Code != http.StatusOK {
		t.Fatalf("verify latest = %d, %s", rec.Code, rec.Body)
	}
}

// users-and-access R51, R43: a limited or failed send still continues to the code step,
// and says no new code went out.
func TestIdentifyStillAnswersTheCodeStepWhenNoNewCodeGoesOut(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addUser("new@example.com", func(u *store.User) { u.PasswordHash = pgtype.Text{} })
	h.identify("new@example.com")
	if got := h.identify("new@example.com"); got != (identifyBody{Next: "first-time-code", CodeDelivery: "suppressed"}) {
		t.Fatalf("within cooldown = %+v, want suppressed", got)
	}
	if len(m.messages) != 1 {
		t.Fatalf("cooldown sent %d emails, want one", len(m.messages))
	}
	h.clock.Advance(time.Minute)
	m.err = errors.New("provider down")
	if got := h.identify("new@example.com"); got != (identifyBody{Next: "first-time-code", CodeDelivery: "failed"}) {
		t.Fatalf("provider failure = %+v, want failed", got)
	}
}

// Without email access only the code step is unavailable; the password step keeps working.
func TestIdentifyAnswersUnavailableOnlyWhereACodeWouldBeSent(t *testing.T) {
	h := newHarness(t)
	h.addUser("staff@example.com")
	h.addUser("new@example.com", func(u *store.User) { u.PasswordHash = pgtype.Text{} })
	if got := h.identify("staff@example.com"); got.Next != "password" {
		t.Fatalf("password step = %+v", got)
	}
	if got := h.identify("nobody@example.com"); got.Next != "no-account" {
		t.Fatalf("unknown = %+v", got)
	}
	rec := h.do(http.MethodPost, "/v2/login/identify", map[string]string{"email": "new@example.com"})
	assertProblem(t, rec, http.StatusServiceUnavailable, "email-auth-unavailable")
}

func (h *harness) identifyFrom(ip, address string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.from("192.0.2.1:1234", http.Header{"X-Vercel-Forwarded-For": {ip}}, http.MethodPost, "/v2/login/identify", `{"email":"`+address+`"}`)
}

// users-and-access R53: 100 checks per IP per rolling hour; over it, a published code and
// a retry time that reveal nothing about the email.
func TestIdentifyLimitsEachIPAddressToOneHundredChecksAnHour(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addUser("staff@example.com")
	h.addUser("new@example.com", func(u *store.User) { u.PasswordHash = pgtype.Text{} })
	for i := range 100 {
		if rec := h.identifyFrom("203.0.113.7", "nobody@example.com"); rec.Code != http.StatusOK {
			t.Fatalf("check %d = %d, %s", i+1, rec.Code, rec.Body)
		}
		h.clock.Advance(time.Second)
	}
	h.clock.Advance(30 * time.Minute)
	var bodies []string
	for _, address := range []string{"staff@example.com", "new@example.com", "nobody@example.com"} {
		rec := h.identifyFrom("203.0.113.7", address)
		assertProblem(t, rec, http.StatusTooManyRequests, "login-check-limited")
		// The oldest check frees at 09:00 + 1h; now is 09:31:40.
		if got := rec.Header().Get("Retry-After"); got != "1700" {
			t.Errorf("Retry-After = %q, want 1700", got)
		}
		bodies = append(bodies, rec.Body.String())
	}
	if bodies[0] != bodies[1] || bodies[1] != bodies[2] {
		t.Fatalf("limited answers differ by email: %q", bodies)
	}
	if len(m.messages) != 0 {
		t.Fatalf("a limited check sent %d emails", len(m.messages))
	}
	if rec := h.identifyFrom("198.51.100.4", "staff@example.com"); rec.Code != http.StatusOK {
		t.Fatalf("another IP = %d, %s", rec.Code, rec.Body)
	}
	h.clock.Set(time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	if rec := h.identifyFrom("203.0.113.7", "staff@example.com"); rec.Code != http.StatusOK {
		t.Fatalf("after the oldest check left the hour = %d, %s", rec.Code, rec.Body)
	}
	assertProblem(t, h.identifyFrom("203.0.113.7", "staff@example.com"), http.StatusTooManyRequests, "login-check-limited")
}

// users-and-access R50, R36: an email check is not a login attempt, so the Login log
// does not record it.
func TestIdentifyIsNotRecordedInTheLoginLog(t *testing.T) {
	h, _ := newEmailHarness(t)
	owner, _ := h.ownerAnd("staff@example.com")
	before := len(h.loginLog("", owner).Entries)
	h.identify("staff@example.com")
	h.identify("nobody@example.com")
	if after := h.loginLog("", owner).Entries; len(after) != before {
		t.Fatalf("login log grew from %d to %d: %+v", before, len(after), after)
	}
}

// An invalid email is refused before the R53 limit counts it.
func TestIdentifyRefusesAnInvalidEmailWithoutCountingIt(t *testing.T) {
	h, _ := newEmailHarness(t)
	for range store.LoginChecksPerHour + 1 {
		rec := h.identifyFrom("203.0.113.7", "not-an-email")
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid email = %d, %s", rec.Code, rec.Body)
		}
	}
	if rec := h.identifyFrom("203.0.113.7", "nobody@example.com"); rec.Code != http.StatusOK {
		t.Fatalf("invalid emails were counted: %d, %s", rec.Code, rec.Body)
	}
}
