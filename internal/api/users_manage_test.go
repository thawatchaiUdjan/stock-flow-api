package api_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/api"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store/storetest"
)

var inactive = map[string]string{"status": "inactive"}

func (h *harness) updateUser(id int64, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPatch, fmt.Sprintf("/users/%d", id), body, cookies...)
}

func (h *harness) resetPassword(id int64, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, fmt.Sprintf("/users/%d/reset-password", id), nil, cookies...)
}

type setupLinkBody struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// ownerAnd seeds the Owner, logs them in, and seeds a Staff User to act on.
func (h *harness) ownerAnd(email string, change ...func(*store.User)) (*http.Cookie, store.User) {
	h.t.Helper()
	h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
	return h.session(ownerSessionEmail), h.addUser(email, change...)
}

// issueLink gives a seeded User a Setup link and returns its token.
func (h *harness) issueLink(user store.User) string {
	h.t.Helper()
	link := auth.NewSetupLink(h.clock.Now())
	err := h.store.IssueSetupLink(h.t.Context(), store.IssueSetupLinkParams{
		UserID: user.ID, TokenHash: auth.HashToken(link.Token), ExpiresAt: link.ExpiresAt,
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return link.Token
}

// users-and-access R35, R14: the name changes, in any language, and nothing else does.
func TestUpdateUserChangesAName(t *testing.T) {
	h := newHarness(t)
	owner, somchai := h.ownerAnd("somchai@example.com")

	rec := h.updateUser(somchai.ID, map[string]string{"name": "สมชาย ใจดี"}, owner)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	want := userBody{
		ID: somchai.ID, Name: "สมชาย ใจดี", Email: "somchai@example.com",
		Role: "staff", Status: "active", PasswordSet: true,
	}
	if got := decode[userBody](t, rec); got != want {
		t.Errorf("user = %+v, want %+v", got, want)
	}
	list := decode[struct {
		Users []userBody `json:"users"`
	}](t, h.listUsers(owner))
	if got := list.Users[len(list.Users)-1]; got != want {
		t.Errorf("listed = %+v, want %+v", got, want)
	}
}

// users-and-access R35: a Role change applies to the next request on every open session,
// with no new login, and to what that Role may reach (R23).
func TestARoleChangeAppliesToEveryOpenSessionAtOnce(t *testing.T) {
	h := newHarness(t)
	owner, napa := h.ownerAnd("napa@example.com", func(u *store.User) { u.Role = "admin" })
	laptop, phone := h.session("napa@example.com"), h.session("napa@example.com")

	rec := h.updateUser(napa.ID, map[string]string{"role": "staff"}, owner)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	if got := decode[userBody](t, rec).Role; got != "staff" {
		t.Errorf("role = %q, want staff", got)
	}
	for name, cookie := range map[string]*http.Cookie{"laptop": laptop, "phone": phone} {
		if got := decode[map[string]any](t, h.me(cookie))["role"]; got != "staff" {
			t.Errorf("%s: role = %v, want staff", name, got)
		}
	}
}

// users-and-access R35: and back the other way, leaving the name alone.
func TestUpdateUserChangesARoleFromStaffToAdmin(t *testing.T) {
	h := newHarness(t)
	owner, somchai := h.ownerAnd("somchai@example.com")

	rec := h.updateUser(somchai.ID, map[string]string{"role": "admin"}, owner)

	want := userBody{
		ID: somchai.ID, Name: "Somchai", Email: "somchai@example.com",
		Role: "admin", Status: "active", PasswordSet: true,
	}
	if got := decode[userBody](t, rec); rec.Code != http.StatusOK || got != want {
		t.Errorf("status = %d, user = %+v, want %+v", rec.Code, got, want)
	}
}

// users-and-access R9, R14, R15, R16, R35: what Update User refuses, each with a field
// error, and without changing the User.
func TestUpdateUserRefusesWhatItCannotChange(t *testing.T) {
	cases := map[string]struct {
		body     any
		location string
	}{
		"an email":        {map[string]string{"email": "other@example.com"}, "body.email"},
		"the Role Owner":  {map[string]string{"role": "owner"}, "body.role"},
		"an unknown Role": {map[string]string{"role": "manager"}, "body.role"},
		"a status":        {map[string]string{"status": "deleted"}, "body.status"},
		"an empty name":   {map[string]string{"name": ""}, "body.name"},
		"a blank name":    {map[string]string{"name": "   "}, "body.name"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			owner, somchai := h.ownerAnd("somchai@example.com")

			rec := h.updateUser(somchai.ID, c.body, owner)

			assertProblem(t, rec, http.StatusUnprocessableEntity, "")
			if got := decode[problem](t, rec); !hasFieldError(got, c.location) {
				t.Errorf("errors = %+v, want one at %s", got.Errors, c.location)
			}
			got, err := h.store.GetUserByEmail(t.Context(), "somchai@example.com")
			if err != nil {
				t.Fatal(err)
			}
			if got != somchai {
				t.Errorf("user = %+v, want it unchanged: %+v", got, somchai)
			}
		})
	}
}

// A body with nothing to change changes nothing: "any of them" includes none. A null is
// no value, so it clears nothing (users-and-access R14).
func TestUpdateUserWithNothingToChangeAnswersTheUserUnchanged(t *testing.T) {
	for name, body := range map[string]map[string]any{
		"an empty body": {},
		"nulls":         {"name": nil, "role": nil, "status": nil},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			owner, somchai := h.ownerAnd("somchai@example.com")

			rec := h.updateUser(somchai.ID, body, owner)

			want := userBody{
				ID: somchai.ID, Name: "Somchai", Email: "somchai@example.com",
				Role: "staff", Status: "active", PasswordSet: true,
			}
			if got := decode[userBody](t, rec); rec.Code != http.StatusOK || got != want {
				t.Errorf("status = %d, user = %+v, want %+v", rec.Code, got, want)
			}
		})
	}
}

// users-and-access R18, context ADR 0004: the Owner's own User is refused whatever is asked,
// so the system can never lose its Owner.
func TestUpdateUserRefusesTheOwnerAsTarget(t *testing.T) {
	for name, body := range map[string]map[string]string{
		"status": {"status": "inactive"},
		"Role":   {"role": "staff"},
		"name":   {"name": "Someone else"},
		"empty":  {},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			malee := h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
			cookie := h.session(ownerSessionEmail)

			rec := h.updateUser(malee.ID, body, cookie)

			assertProblem(t, rec, http.StatusForbidden, "target-is-owner")
			got, err := h.store.GetUserByEmail(t.Context(), ownerSessionEmail)
			if err != nil {
				t.Fatal(err)
			}
			if got != malee {
				t.Errorf("owner = %+v, want it unchanged: %+v", got, malee)
			}
			if rec := h.me(cookie); rec.Code != http.StatusOK {
				t.Errorf("the Owner's session: status = %d, want 200", rec.Code)
			}
		})
	}
}

func TestUpdateUserAnswers404ForNoSuchUser(t *testing.T) {
	h := newHarness(t)
	owner, _ := h.ownerAnd("somchai@example.com")

	rec := h.updateUser(404, map[string]string{"name": "Nobody"}, owner)

	assertProblem(t, rec, http.StatusNotFound, "")
}

// users-and-access R16: every session ends at once, on every device, and no one else's.
func TestSettingAUserInactiveEndsAllTheirSessionsAtOnce(t *testing.T) {
	h := newHarness(t)
	owner, somchai := h.ownerAnd("somchai@example.com")
	h.addUser("other@example.com")
	laptop, phone := h.session("somchai@example.com"), h.session("somchai@example.com")
	other := h.session("other@example.com")

	rec := h.updateUser(somchai.ID, inactive, owner)

	if got := decode[userBody](t, rec); rec.Code != http.StatusOK || got.Status != "inactive" {
		t.Fatalf("status = %d, user = %+v", rec.Code, got)
	}
	assertProblem(t, h.me(laptop), http.StatusUnauthorized, "session-ended")
	assertProblem(t, h.me(phone), http.StatusUnauthorized, "session-ended")
	if rec := h.me(other); rec.Code != http.StatusOK {
		t.Errorf("another User's session: status = %d, want 200", rec.Code)
	}
	if rec := h.me(owner); rec.Code != http.StatusOK {
		t.Errorf("the Owner's session: status = %d, want 200", rec.Code)
	}
}

// users-and-access R16, R5: the right password fails like any other failure.
func TestAnInactiveUserCannotLogIn(t *testing.T) {
	h := newHarness(t)
	owner, somchai := h.ownerAnd("somchai@example.com")
	h.updateUser(somchai.ID, inactive, owner)

	assertProblem(t, h.logIn("somchai@example.com", password), http.StatusUnauthorized, "login-failed")
}

// users-and-access R32.
func TestSettingAUserInactiveStopsTheirSetupLinkAtOnce(t *testing.T) {
	h := newHarness(t)
	h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
	owner := h.session(ownerSessionEmail)
	created := decode[createdUser](t, h.createUser(newUser("Napa", "napa@example.com", "staff"), owner))

	h.updateUser(created.User.ID, inactive, owner)

	assertProblem(t, h.readSetupLink(created.SetupLink.Token), http.StatusGone, "setup-link-invalid")
	assertProblem(t, h.setPassword(created.SetupLink.Token, password), http.StatusGone, "setup-link-invalid")
}

// users-and-access R16, R17: a returning person keeps their User, password and Role, and
// logs in again. What Inactive ended stays ended.
func TestSettingAUserActiveAgainRestoresTheirPasswordAndRoleButNoSession(t *testing.T) {
	h := newHarness(t)
	owner, napa := h.ownerAnd("napa@example.com", func(u *store.User) { u.Role = "admin" })
	before := h.session("napa@example.com")
	h.updateUser(napa.ID, inactive, owner)

	rec := h.updateUser(napa.ID, map[string]string{"status": "active"}, owner)

	want := userBody{
		ID: napa.ID, Name: "Somchai", Email: "napa@example.com",
		Role: "admin", Status: "active", PasswordSet: true,
	}
	if got := decode[userBody](t, rec); rec.Code != http.StatusOK || got != want {
		t.Errorf("status = %d, user = %+v, want %+v", rec.Code, got, want)
	}
	assertProblem(t, h.me(before), http.StatusUnauthorized, "session-ended")
	if got := decode[map[string]any](t, h.me(h.session("napa@example.com")))["role"]; got != "admin" {
		t.Errorf("role = %v, want admin", got)
	}
}

// users-and-access R32: a Setup link stopped by Inactive doesn't come back with Active.
func TestSettingAUserActiveAgainBringsNoSetupLinkBack(t *testing.T) {
	h := newHarness(t)
	h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
	owner := h.session(ownerSessionEmail)
	created := decode[createdUser](t, h.createUser(newUser("Napa", "napa@example.com", "staff"), owner))
	h.updateUser(created.User.ID, inactive, owner)

	h.updateUser(created.User.ID, map[string]string{"status": "active"}, owner)

	assertProblem(t, h.readSetupLink(created.SetupLink.Token), http.StatusGone, "setup-link-invalid")
}

// users-and-access R16, R35: only Inactive ends anything. A new name, a new Role or an
// Active that was Active already leave the sessions and the Setup link alone.
func TestAChangeThatIsNotInactiveEndsNothing(t *testing.T) {
	for name, body := range map[string]map[string]string{
		"a name":     {"name": "Napa"},
		"a Role":     {"role": "admin"},
		"Active":     {"status": "active"},
		"everything": {"name": "Napa", "role": "admin", "status": "active"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			owner, somchai := h.ownerAnd("somchai@example.com")
			cookie := h.session("somchai@example.com")
			token := h.issueLink(somchai)

			rec := h.updateUser(somchai.ID, body, owner)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
			}
			if rec := h.me(cookie); rec.Code != http.StatusOK {
				t.Errorf("session: status = %d, want 200", rec.Code)
			}
			if rec := h.readSetupLink(token); rec.Code != http.StatusOK {
				t.Errorf("Setup link: status = %d, want 200", rec.Code)
			}
		})
	}
}

// users-and-access R31: the password stops working, every session ends, and the new
// Setup link is what lets the person back in (R20, R27).
func TestResetPasswordEndsThePasswordAndSessionsAndAnswersANewSetupLink(t *testing.T) {
	h := newHarness(t)
	owner, somchai := h.ownerAnd("somchai@example.com")
	h.addUser("other@example.com")
	laptop, phone := h.session("somchai@example.com"), h.session("somchai@example.com")
	other := h.session("other@example.com")

	rec := h.resetPassword(somchai.ID, owner)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	link := decode[setupLinkBody](t, rec)
	if link.Token == "" {
		t.Error("no Setup link token")
	}
	if want := h.clock.Now().Add(24 * time.Hour); !link.ExpiresAt.Equal(want) {
		t.Errorf("expiry = %v, want %v", link.ExpiresAt, want)
	}
	assertProblem(t, h.logIn("somchai@example.com", password), http.StatusUnauthorized, "login-failed")
	assertProblem(t, h.me(laptop), http.StatusUnauthorized, "session-ended")
	assertProblem(t, h.me(phone), http.StatusUnauthorized, "session-ended")
	if rec := h.me(other); rec.Code != http.StatusOK {
		t.Errorf("another User's session: status = %d, want 200", rec.Code)
	}
	list := decode[struct {
		Users []userBody `json:"users"`
	}](t, h.listUsers(owner))
	for _, user := range list.Users {
		if want := user.Email != "somchai@example.com"; user.PasswordSet != want {
			t.Errorf("%s: passwordSet = %v, want %v", user.Email, user.PasswordSet, want)
		}
	}

	const newPassword = "a new password"
	if rec := h.setPassword(link.Token, newPassword); rec.Code != http.StatusNoContent {
		t.Fatalf("set password: status = %d; body %s", rec.Code, rec.Body)
	}
	if rec := h.logIn("somchai@example.com", newPassword); rec.Code != http.StatusOK {
		t.Errorf("log in with the new password: status = %d; body %s", rec.Code, rec.Body)
	}
}

// users-and-access R29: any earlier link stops working at once.
func TestResetPasswordCancelsEarlierSetupLinks(t *testing.T) {
	h := newHarness(t)
	owner, somchai := h.ownerAnd("somchai@example.com")
	first := h.issueLink(somchai)
	second := decode[setupLinkBody](t, h.resetPassword(somchai.ID, owner)).Token

	third := decode[setupLinkBody](t, h.resetPassword(somchai.ID, owner)).Token

	assertProblem(t, h.readSetupLink(first), http.StatusGone, "setup-link-invalid")
	assertProblem(t, h.readSetupLink(second), http.StatusGone, "setup-link-invalid")
	if rec := h.readSetupLink(third); rec.Code != http.StatusOK {
		t.Errorf("newest link: status = %d; body %s", rec.Code, rec.Body)
	}
}

// users-and-access R29, and the spec's "One operation for R29 and R31": for a User with no
// password yet, all that happens is a new link.
func TestResetPasswordIssuesANewSetupLinkForAUserWithNoPasswordYet(t *testing.T) {
	h := newHarness(t)
	h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
	owner := h.session(ownerSessionEmail)
	created := decode[createdUser](t, h.createUser(newUser("Napa", "napa@example.com", "staff"), owner))
	h.clock.Advance(25 * time.Hour) // the first link has expired; the 7-day session has not

	rec := h.resetPassword(created.User.ID, owner)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	if rec := h.setPassword(decode[setupLinkBody](t, rec).Token, password); rec.Code != http.StatusNoContent {
		t.Errorf("set password: status = %d; body %s", rec.Code, rec.Body)
	}
	if rec := h.logIn("napa@example.com", password); rec.Code != http.StatusOK {
		t.Errorf("log in: status = %d; body %s", rec.Code, rec.Body)
	}
}

// users-and-access R32: no Setup link can be issued for an Inactive User, and the refusal
// changes nothing, so setting them Active again still restores their password (R16).
func TestResetPasswordRefusesAnInactiveUser(t *testing.T) {
	h := newHarness(t)
	owner, somchai := h.ownerAnd("somchai@example.com")
	h.updateUser(somchai.ID, inactive, owner)

	rec := h.resetPassword(somchai.ID, owner)

	assertProblem(t, rec, http.StatusConflict, "user-inactive")
	if links := h.store.SetupLinks(); len(links) != 0 {
		t.Errorf("stored links = %d, want 0", len(links))
	}
	h.updateUser(somchai.ID, map[string]string{"status": "active"}, owner)
	if rec := h.logIn("somchai@example.com", password); rec.Code != http.StatusOK {
		t.Errorf("log in: status = %d; body %s", rec.Code, rec.Body)
	}
}

// users-and-access R34, and the ticket: the Owner's password is not reset here.
func TestResetPasswordRefusesTheOwnerAsTarget(t *testing.T) {
	h := newHarness(t)
	malee := h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
	cookie := h.session(ownerSessionEmail)

	rec := h.resetPassword(malee.ID, cookie)

	assertProblem(t, rec, http.StatusForbidden, "target-is-owner")
	if rec := h.me(cookie); rec.Code != http.StatusOK {
		t.Errorf("the Owner's session: status = %d, want 200", rec.Code)
	}
	if rec := h.logIn(ownerSessionEmail, password); rec.Code != http.StatusOK {
		t.Errorf("the Owner's password: status = %d; body %s", rec.Code, rec.Body)
	}
	if links := h.store.SetupLinks(); len(links) != 0 {
		t.Errorf("stored links = %d, want 0", len(links))
	}
}

func TestResetPasswordAnswers404ForNoSuchUser(t *testing.T) {
	h := newHarness(t)
	owner, _ := h.ownerAnd("somchai@example.com")

	assertProblem(t, h.resetPassword(404, owner), http.StatusNotFound, "")
}

// context ADR 0002: the token is shown once and only its hash is stored.
func TestResetPasswordStoresOnlyTheHashOfTheToken(t *testing.T) {
	h := newHarness(t)
	owner, somchai := h.ownerAnd("somchai@example.com")

	token := decode[setupLinkBody](t, h.resetPassword(somchai.ID, owner)).Token

	links := h.store.SetupLinks()
	if len(links) != 1 || !bytes.Equal(links[0].TokenHash, auth.HashToken(token)) {
		t.Errorf("stored %+v, want one link with the hash of the token", links)
	}
}

// users-and-access R16, R31: a login checks the password slowly. What the Owner does in
// that time wins, so nobody ends up logged in with access the Owner just ended.
func TestALoginUnderWayStartsNoSessionWhenTheOwnerEndsTheUsersAccess(t *testing.T) {
	for name, end := range map[string]func(*harness, store.User, *http.Cookie){
		"Inactive": func(h *harness, user store.User, owner *http.Cookie) { h.updateUser(user.ID, inactive, owner) },
		"reset":    func(h *harness, user store.User, owner *http.Cookie) { h.resetPassword(user.ID, owner) },
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			owner, somchai := h.ownerAnd("somchai@example.com")
			h.whileVerifying = func() { end(h, somchai, owner) }

			rec := h.logIn("somchai@example.com", password)

			assertProblem(t, rec, http.StatusUnauthorized, "login-failed")
			if len(rec.Result().Cookies()) != 0 {
				t.Errorf("cookies = %v, want none", rec.Result().Cookies())
			}
		})
	}
}

// failingChanges is a store that logs in and reads as usual, and fails only these two, so
// the failure reaches the operations and not their access check.
type failingChanges struct {
	*storetest.Fake
	err error
}

func (f failingChanges) UpdateUser(context.Context, store.ChangeUserParams) (store.ChangeUserRow, error) {
	return store.ChangeUserRow{}, f.err
}

func (f failingChanges) ResetPassword(context.Context, store.IssueSetupLinkParams) error {
	return f.err
}

func TestUpdateUserAndResetPasswordAnswerAStoreFailureWithoutItsCause(t *testing.T) {
	h := newHarness(t)
	owner, somchai := h.ownerAnd("somchai@example.com")
	h.handler, _ = api.New(api.Deps{
		Store:          failingChanges{h.store, errors.New("connection to db.internal:5432 refused")},
		PasswordHasher: cheap,
		Now:            h.clock.Now,
	})

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"update user":    h.updateUser(somchai.ID, inactive, owner),
		"reset password": h.resetPassword(somchai.ID, owner),
	} {
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, want 500; body %s", name, rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), "db.internal") {
			t.Errorf("%s: body leaks the cause: %s", name, rec.Body)
		}
	}
}

func hasFieldError(got problem, location string) bool {
	for _, e := range got.Errors {
		if e.Location == location {
			return true
		}
	}
	return false
}
