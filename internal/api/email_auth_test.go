package api_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/api"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/email"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

type mailbox struct {
	messages []email.Message
	err      error
}

func (h *harness) requestCode(address string) {
	h.t.Helper()
	rec := h.do(http.MethodPost, "/v2/password-setup/request", map[string]string{"email": address})
	if rec.Code != http.StatusAccepted {
		h.t.Fatalf("request code = %d, %s", rec.Code, rec.Body)
	}
}

func (h *harness) verifyCode(address, code string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, "/v2/password-setup/verify", map[string]string{"email": address, "code": code})
}

// users-and-access R41: resends preserve both failures and the ten-minute block.
func TestFiveWrongCodesBlockVerificationAcrossResendsForTenMinutes(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addUser("new@example.com", func(u *store.User) { u.PasswordHash = pgtype.Text{} })
	h.requestCode("new@example.com")
	wrong := "000000"
	if m.messages[0].Code == wrong {
		wrong = "111111"
	}
	for range 4 {
		assertProblem(t, h.verifyCode("new@example.com", wrong), http.StatusGone, "first-time-code-invalid")
	}
	h.clock.Advance(time.Minute)
	h.requestCode("NEW@example.com")
	latest := m.messages[len(m.messages)-1].Code
	wrong = "000000"
	if latest == wrong {
		wrong = "111111"
	}
	assertProblem(t, h.verifyCode("new@example.com", wrong), http.StatusGone, "first-time-code-invalid")
	assertProblem(t, h.verifyCode("new@example.com", latest), http.StatusGone, "first-time-code-invalid")
	h.clock.Advance(time.Minute)
	h.requestCode("new@example.com")
	latest = m.messages[len(m.messages)-1].Code
	assertProblem(t, h.verifyCode("new@example.com", latest), http.StatusGone, "first-time-code-invalid")
	h.clock.Advance(9 * time.Minute)
	if rec := h.verifyCode("new@example.com", latest); rec.Code != http.StatusOK {
		t.Fatalf("after block expiry = %d, %s", rec.Code, rec.Body)
	}
}

// users-and-access R43, R44: the cooldown is shared by letter-case variants.
func TestEmailRequestsRespectSixtySecondCooldownAndRollingHour(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addUser("new@example.com", func(u *store.User) { u.PasswordHash = pgtype.Text{} })
	h.requestCode("new@example.com")
	first := h.do(http.MethodPost, "/v2/password-setup/request", map[string]string{"email": "NEW@example.com"})
	if len(m.messages) != 1 {
		t.Fatalf("cooldown delivered %d emails, want one", len(m.messages))
	}
	h.clock.Advance(59 * time.Second)
	h.requestCode("new@example.com")
	if len(m.messages) != 1 {
		t.Fatal("email sent before sixty seconds")
	}
	h.clock.Advance(time.Second)
	for range 4 {
		h.requestCode("NEW@example.com")
		h.clock.Advance(time.Minute)
	}
	if len(m.messages) != 5 {
		t.Fatalf("within allowance = %d, want five", len(m.messages))
	}
	last := h.do(http.MethodPost, "/v2/password-setup/request", map[string]string{"email": "new@example.com"})
	if len(m.messages) != 5 || first.Body.String() != last.Body.String() {
		t.Fatalf("hour limit reveals outcome or sends a sixth email: %d", len(m.messages))
	}
	h.clock.Advance(55*time.Minute - time.Second)
	h.requestCode("new@example.com")
	if len(m.messages) != 5 {
		t.Fatal("hour allowance reset early")
	}
	h.clock.Advance(time.Second)
	h.requestCode("new@example.com")
	if len(m.messages) != 6 {
		t.Fatal("oldest request did not leave the rolling hour")
	}
}

// users-and-access R43: many addresses cannot bypass the shared IP allowance.
func TestEmailRequestsLimitThirtyPerIPPerHour(t *testing.T) {
	h, m := newEmailHarness(t)
	for i := range 31 {
		address := fmt.Sprintf("user%d@example.com", i)
		h.addUser(address, func(u *store.User) { u.PasswordHash = pgtype.Text{} })
		h.requestCode(address)
	}
	if len(m.messages) != 30 {
		t.Fatalf("same-IP deliveries = %d, want thirty", len(m.messages))
	}
	h.clock.Advance(time.Hour)
	h.requestCode("user30@example.com")
	if len(m.messages) != 31 {
		t.Fatal("IP window did not expire")
	}
}

// users-and-access R27, R42, R48: exact expiry boundaries and consumption.
func TestEmailSetupCredentialsExpireAtTheirBoundaryAndWorkOnce(t *testing.T) {
	for _, elapsed := range []time.Duration{10*time.Minute - time.Nanosecond, 10 * time.Minute} {
		t.Run(fmt.Sprint(elapsed), func(t *testing.T) {
			h, m := newEmailHarness(t)
			h.addUser("new@example.com", func(u *store.User) { u.PasswordHash = pgtype.Text{} })
			h.requestCode("new@example.com")
			h.clock.Advance(elapsed)
			verified := h.verifyCode("new@example.com", m.messages[0].Code)
			if elapsed == 10*time.Minute {
				assertProblem(t, verified, http.StatusGone, "first-time-code-invalid")
				return
			}
			if verified.Code != http.StatusOK {
				t.Fatalf("verify = %d, %s", verified.Code, verified.Body)
			}
			grant := decode[verificationBody](t, verified)
			assertProblem(t, h.verifyCode("new@example.com", m.messages[0].Code), http.StatusGone, "first-time-code-invalid")
			h.clock.Advance(10 * time.Minute)
			assertProblem(t, h.do(http.MethodPost, "/v2/password-setup/password", map[string]string{"token": grant.Token, "password": password}), http.StatusGone, "password-setup-invalid")
		})
	}
	h, m := newEmailHarness(t)
	h.addUser("once@example.com", func(u *store.User) { u.PasswordHash = pgtype.Text{} })
	h.requestCode("once@example.com")
	grant := decode[verificationBody](t, h.verifyCode("once@example.com", m.messages[0].Code))
	save := func() *httptest.ResponseRecorder {
		return h.do(http.MethodPost, "/v2/password-setup/password", map[string]string{"token": grant.Token, "password": password})
	}
	if rec := save(); rec.Code != http.StatusNoContent {
		t.Fatalf("save = %d, %s", rec.Code, rec.Body)
	}
	assertProblem(t, save(), http.StatusGone, "password-setup-invalid")
}

// users-and-access R32: reactivation must not revive an old code or verified grant.
func TestReactivationNeverRevivesEmailSetupCredentials(t *testing.T) {
	for _, verified := range []bool{false, true} {
		t.Run(fmt.Sprint(verified), func(t *testing.T) {
			h, m := newEmailHarness(t)
			owner, target := h.ownerAnd("new@example.com", func(u *store.User) { u.PasswordHash = pgtype.Text{} })
			h.requestCode(target.Email)
			code, token := m.messages[0].Code, ""
			if verified {
				token = decode[verificationBody](t, h.verifyCode(target.Email, code)).Token
			}
			for _, status := range []string{"inactive", "active"} {
				if rec := h.updateUser(target.ID, map[string]string{"status": status}, owner); rec.Code != http.StatusOK {
					t.Fatalf("status = %d, %s", rec.Code, rec.Body)
				}
			}
			if verified {
				assertProblem(t, h.do(http.MethodPost, "/v2/password-setup/password", map[string]string{"token": token, "password": password}), http.StatusGone, "password-setup-invalid")
			} else {
				assertProblem(t, h.verifyCode(target.Email, code), http.StatusGone, "first-time-code-invalid")
			}
		})
	}
}

// users-and-access R34, R44, R46: public responses never expose eligibility/delivery.
func TestPublicSetupRequestsHideEligibilityAndEmailFailure(t *testing.T) {
	h, m := newEmailHarness(t)
	h.installOwner()
	h.addUser("inactive@example.com", func(u *store.User) { u.Status = "inactive"; u.PasswordHash = pgtype.Text{} })
	h.addUser("password@example.com")
	h.addUser("new@example.com", func(u *store.User) { u.PasswordHash = pgtype.Text{} })
	m.err = errors.New("provider rejected secret-token")
	var first string
	for _, address := range []string{"missing@example.com", ownerEmail, "inactive@example.com", "password@example.com", "new@example.com", "NEW@example.com"} {
		rec := h.do(http.MethodPost, "/v2/password-setup/request", map[string]string{"email": address})
		if rec.Code != http.StatusAccepted {
			t.Fatalf("%s = %d, %s", address, rec.Code, rec.Body)
		}
		if first == "" {
			first = rec.Body.String()
		}
		if rec.Body.String() != first {
			t.Fatalf("%s reveals request outcome: %s", address, rec.Body)
		}
	}
	if len(m.messages) != 0 {
		t.Fatal("ineligible or failed requests delivered instructions")
	}
}

func (m *mailbox) Send(_ context.Context, message email.Message) error {
	if m.err != nil {
		return m.err
	}
	m.messages = append(m.messages, message)
	return nil
}

func newEmailHarness(t *testing.T) (*harness, *mailbox) {
	t.Helper()
	h := newHarness(t)
	m := &mailbox{}
	h.handler, h.contract = api.New(api.Deps{
		Store: h.store, PasswordHasher: cheap, Now: h.clock.Now,
		Mailer: m, CodeSecret: []byte("test-secret-with-at-least-32-bytes"), WebBaseURL: "https://stock-flow.example",
	})
	return h, m
}

type emailUserBody struct {
	User          userBody `json:"user"`
	EmailDelivery string   `json:"emailDelivery"`
}

type verificationBody struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// users-and-access R2, R10, R20, R27-R30, R42: the first-time setup tracer bullet.
func TestEmailCreatedUserVerifiesCodeSetsOwnPasswordAndLogsIn(t *testing.T) {
	h, m := newEmailHarness(t)
	owner, _ := h.ownerAnd("existing@example.com")
	created := h.do(http.MethodPost, "/v2/users", map[string]string{
		"name": "Napa", "email": "Napa@Example.com", "role": "staff",
	}, owner)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d, %s", created.Code, created.Body)
	}
	body := decode[emailUserBody](t, created)
	if body.EmailDelivery != "sent" || body.User.Name != "Napa" || body.User.PasswordSet {
		t.Fatalf("created = %+v", body)
	}
	if len(m.messages) != 1 {
		t.Fatalf("received %d messages, want one", len(m.messages))
	}
	message := m.messages[0]
	if message.To != "Napa@Example.com" || message.Kind != email.FirstTimeCode || !regexp.MustCompile(`^[0-9]{6}$`).MatchString(message.Code) {
		t.Fatalf("first-time message = %+v", message)
	}
	if strings.Contains(created.Body.String(), message.Code) || strings.Contains(created.Body.String(), "setupLink") {
		t.Fatal("creation exposes password-setup credentials")
	}
	verified := h.do(http.MethodPost, "/v2/password-setup/verify", map[string]string{
		"email": "napa@example.com", "code": message.Code,
	})
	if verified.Code != http.StatusOK {
		t.Fatalf("verify = %d, %s", verified.Code, verified.Body)
	}
	grant := decode[verificationBody](t, verified)
	if grant.Token == "" || !grant.ExpiresAt.Equal(h.clock.Now().Add(10*time.Minute)) {
		t.Fatalf("grant = %+v", grant)
	}
	saved := h.do(http.MethodPost, "/v2/password-setup/password", map[string]string{
		"token": grant.Token, "password": "my own password",
	})
	if saved.Code != http.StatusNoContent || saved.Header().Get("Set-Cookie") != "" {
		t.Fatalf("save = %d, cookies = %s, body = %s", saved.Code, saved.Header().Get("Set-Cookie"), saved.Body)
	}
	if loggedIn := h.logIn("napa@example.com", "my own password"); loggedIn.Code != http.StatusOK {
		t.Fatalf("login = %d, %s", loggedIn.Code, loggedIn.Body)
	}
}
