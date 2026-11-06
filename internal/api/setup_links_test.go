package api_test

import (
	"bytes"
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
)

const ownerEmail = "malee@example.com"

// installOwner runs what the developer command runs and returns the Setup link token.
func (h *harness) installOwner() string {
	h.t.Helper()
	link, err := auth.IssueOwnerSetupLink(context.Background(), h.store, h.clock.Now(), "Malee", ownerEmail)
	if err != nil {
		h.t.Fatal(err)
	}
	return link.Token
}

func (h *harness) readSetupLink(token string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, "/setup-link/read", map[string]string{"token": token})
}

func (h *harness) setPassword(token, password string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, "/setup-link/password", map[string]string{"token": token, "password": password})
}

func TestReadSetupLinkAnswersWhoItIsFor(t *testing.T) {
	h := newHarness(t)

	rec := h.readSetupLink(h.installOwner())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	body := decode[map[string]any](t, rec)
	delete(body, "$schema")
	if want := (map[string]any{"name": "Malee", "email": ownerEmail}); !maps.Equal(body, want) {
		t.Errorf("body = %v, want %v", body, want)
	}
}

// users-and-access R2, R20, R27: the whole path of the ticket.
func TestAPasswordSetThroughASetupLinkLogsIn(t *testing.T) {
	h := newHarness(t)
	token := h.installOwner()
	assertProblem(t, h.logIn(ownerEmail, password), http.StatusUnauthorized, "login-failed")

	rec := h.setPassword(token, password)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("set password: status = %d, want 204; body %s", rec.Code, rec.Body)
	}
	login := h.logIn(ownerEmail, password)
	if login.Code != http.StatusOK {
		t.Fatalf("log in: status = %d, want 200; body %s", login.Code, login.Body)
	}
	me := h.me(sessionCookie(t, login))
	if got := decode[map[string]any](t, me)["role"]; got != "owner" {
		t.Errorf("role = %v, want owner", got)
	}
}

// users-and-access R30: the person goes to Login, so no session starts here.
func TestSetPasswordStartsNoSession(t *testing.T) {
	h := newHarness(t)

	rec := h.setPassword(h.installOwner(), password)

	if got := rec.Header().Get("Set-Cookie"); got != "" {
		t.Errorf("Set-Cookie = %q, want none", got)
	}
}

// users-and-access R27.
func TestASetupLinkWorksOnce(t *testing.T) {
	h := newHarness(t)
	token := h.installOwner()
	if rec := h.setPassword(token, password); rec.Code != http.StatusNoContent {
		t.Fatalf("first use: status = %d; body %s", rec.Code, rec.Body)
	}

	assertProblem(t, h.setPassword(token, "another password"), http.StatusGone, "setup-link-invalid")
	assertProblem(t, h.readSetupLink(token), http.StatusGone, "setup-link-invalid")
	assertProblem(t, h.logIn(ownerEmail, "another password"), http.StatusUnauthorized, "login-failed")
	if rec := h.logIn(ownerEmail, password); rec.Code != http.StatusOK {
		t.Errorf("first password: status = %d, want 200; body %s", rec.Code, rec.Body)
	}
}

// users-and-access R27.
func TestASetupLinkExpires24HoursAfterItIsIssued(t *testing.T) {
	t.Run("a second before", func(t *testing.T) {
		h := newHarness(t)
		token := h.installOwner()
		h.clock.Advance(24*time.Hour - time.Second)

		if rec := h.readSetupLink(token); rec.Code != http.StatusOK {
			t.Errorf("read: status = %d, want 200; body %s", rec.Code, rec.Body)
		}
		if rec := h.setPassword(token, password); rec.Code != http.StatusNoContent {
			t.Errorf("set password: status = %d, want 204; body %s", rec.Code, rec.Body)
		}
	})

	t.Run("at 24 hours", func(t *testing.T) {
		h := newHarness(t)
		token := h.installOwner()
		h.clock.Advance(24 * time.Hour)

		assertProblem(t, h.readSetupLink(token), http.StatusGone, "setup-link-invalid")
		assertProblem(t, h.setPassword(token, password), http.StatusGone, "setup-link-invalid")
	})
}

// context ADR 0002: only the newest link works.
func TestRunningTheInstallAgainStopsTheEarlierSetupLink(t *testing.T) {
	h := newHarness(t)
	earlier := h.installOwner()

	newest := h.installOwner()

	assertProblem(t, h.readSetupLink(earlier), http.StatusGone, "setup-link-invalid")
	assertProblem(t, h.setPassword(earlier, password), http.StatusGone, "setup-link-invalid")
	if rec := h.setPassword(newest, password); rec.Code != http.StatusNoContent {
		t.Errorf("newest link: status = %d, want 204; body %s", rec.Code, rec.Body)
	}
}

func TestAnUnknownSetupLinkIsGone(t *testing.T) {
	h := newHarness(t)

	assertProblem(t, h.readSetupLink("no-such-token"), http.StatusGone, "setup-link-invalid")
	assertProblem(t, h.setPassword("no-such-token", password), http.StatusGone, "setup-link-invalid")
}

// users-and-access R19.
func TestAPasswordNeedsAtLeastSixCharacters(t *testing.T) {
	refused := map[string]string{"empty": "", "five letters": "abcde", "five Thai characters": "กขคงจ"}
	for name, short := range refused {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			token := h.installOwner()

			rec := h.setPassword(token, short)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422; body %s", rec.Code, rec.Body)
			}
			got := decode[problem](t, rec)
			if len(got.Errors) != 1 || got.Errors[0].Location != "body.password" {
				t.Errorf("errors = %+v, want one at body.password", got.Errors)
			}
			// A refused password doesn't use up the link.
			if rec := h.setPassword(token, password); rec.Code != http.StatusNoContent {
				t.Errorf("then a good password: status = %d, want 204; body %s", rec.Code, rec.Body)
			}
		})
	}

	accepted := map[string]string{"six letters": "abcdef", "six Thai characters": "กขคงจฉ"}
	for name, enough := range accepted {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)

			if rec := h.setPassword(h.installOwner(), enough); rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want 204; body %s", rec.Code, rec.Body)
			}
			if rec := h.logIn(ownerEmail, enough); rec.Code != http.StatusOK {
				t.Errorf("log in: status = %d, want 200; body %s", rec.Code, rec.Body)
			}
		})
	}
}

// context ADR 0002: reading the database does not reveal the token.
func TestASetupLinkTokenIsNotStoredAsIs(t *testing.T) {
	h := newHarness(t)

	token := h.installOwner()

	links := h.store.SetupLinks()
	if len(links) != 1 {
		t.Fatalf("stored %d links, want 1", len(links))
	}
	if stored := links[0].TokenHash; bytes.Contains(stored, []byte(token)) || strings.Contains(string(stored), token) {
		t.Errorf("stored %q contains the token %q", stored, token)
	}
}

// users-and-access R20: nobody sees a password, so nothing answers with its hash.
func TestNoAnswerCarriesAPasswordHash(t *testing.T) {
	h := newHarness(t)
	token := h.installOwner()
	read := h.readSetupLink(token)
	h.setPassword(token, password)
	login := h.logIn(ownerEmail, password)
	me := h.me(sessionCookie(t, login))

	for name, rec := range map[string]*httptest.ResponseRecorder{"read setup link": read, "log in": login, "me": me} {
		if strings.Contains(rec.Body.String(), "argon2id") {
			t.Errorf("%s: body = %s", name, rec.Body)
		}
	}
}
