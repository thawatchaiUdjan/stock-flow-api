package api_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

const ownerSessionEmail = "owner@example.com"

// loggedInAs seeds a User with the Role and logs them in.
func (h *harness) loggedInAs(role string) *http.Cookie {
	h.t.Helper()
	email := role + "@example.com"
	h.addUser(email, func(u *store.User) { u.Role = role })
	return h.session(email)
}

func (h *harness) listUsers(cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodGet, "/users", nil, cookies...)
}

func (h *harness) createUser(body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, "/users", body, cookies...)
}

type userBody struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	PasswordSet bool   `json:"passwordSet"`
}

type createdUser struct {
	User      userBody `json:"user"`
	SetupLink struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expiresAt"`
	} `json:"setupLink"`
}

func newUser(name, email, role string) map[string]string {
	return map[string]string{"name": name, "email": email, "role": role}
}

// users-and-access R33, R14, R8: each User's name, email, Role, status and whether a
// password is set; never the password itself.
func TestListUsersAnswersEachUsersDetailsAndWhetherAPasswordIsSet(t *testing.T) {
	h := newHarness(t)
	owner := h.addUser(ownerSessionEmail, func(u *store.User) { u.Name, u.Role = "Malee", "owner" })
	admin := h.addUser("admin@example.com", func(u *store.User) {
		u.Name, u.Role, u.PasswordHash = "Napa", "admin", pgtype.Text{}
	})
	staff := h.addUser("staff@example.com", func(u *store.User) { u.Name, u.Status = "สมชาย", "inactive" })

	rec := h.listUsers(h.session(ownerSessionEmail))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	got := decode[struct {
		Users []userBody `json:"users"`
	}](t, rec).Users
	want := []userBody{
		{ID: owner.ID, Name: "Malee", Email: ownerSessionEmail, Role: "owner", Status: "active", PasswordSet: true},
		{ID: admin.ID, Name: "Napa", Email: "admin@example.com", Role: "admin", Status: "active", PasswordSet: false},
		{ID: staff.ID, Name: "สมชาย", Email: "staff@example.com", Role: "staff", Status: "inactive", PasswordSet: true},
	}
	if len(got) != len(want) {
		t.Fatalf("users = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("user %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if strings.Contains(rec.Body.String(), "argon2") || strings.Contains(rec.Body.String(), "password_hash") {
		t.Errorf("the list leaks a password hash: %s", rec.Body)
	}
}

// users-and-access R10, R28.
func TestCreateUserAnswersTheUserAndASetupLinkTokenWithItsExpiry(t *testing.T) {
	for _, role := range []string{"admin", "staff"} {
		t.Run(role, func(t *testing.T) {
			h := newHarness(t)
			h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
			owner := h.session(ownerSessionEmail)

			rec := h.createUser(newUser("Napa", "napa@example.com", role), owner)

			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body)
			}
			got := decode[createdUser](t, rec)
			want := userBody{ID: got.User.ID, Name: "Napa", Email: "napa@example.com", Role: role, Status: "active", PasswordSet: false}
			if got.User != want || got.User.ID == 0 {
				t.Errorf("user = %+v, want %+v", got.User, want)
			}
			if got.SetupLink.Token == "" {
				t.Error("no Setup link token")
			}
			if wantExpiry := h.clock.Now().Add(24 * time.Hour); !got.SetupLink.ExpiresAt.Equal(wantExpiry) {
				t.Errorf("expiry = %v, want %v", got.SetupLink.ExpiresAt, wantExpiry)
			}
		})
	}
}

// users-and-access R27, R28, R20: the token the Owner is given is the one that sets the
// new User's password, and the new User can then log in with it.
func TestTheSetupLinkOfANewUserSetsTheirPassword(t *testing.T) {
	h := newHarness(t)
	h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
	created := decode[createdUser](t, h.createUser(newUser("Napa", "napa@example.com", "admin"), h.session(ownerSessionEmail)))

	read := h.readSetupLink(created.SetupLink.Token)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), "napa@example.com") {
		t.Fatalf("read link: status = %d; body %s", read.Code, read.Body)
	}
	if rec := h.setPassword(created.SetupLink.Token, password); rec.Code != http.StatusNoContent {
		t.Fatalf("set password: status = %d; body %s", rec.Code, rec.Body)
	}

	login := h.logIn("napa@example.com", password)
	if login.Code != http.StatusOK {
		t.Fatalf("log in: status = %d; body %s", login.Code, login.Body)
	}
	if got := decode[map[string]any](t, h.me(sessionCookie(t, login)))["role"]; got != "admin" {
		t.Errorf("role = %v, want admin", got)
	}
	// The list now shows the password as set.
	list := decode[struct {
		Users []userBody `json:"users"`
	}](t, h.listUsers(h.session(ownerSessionEmail)))
	if last := list.Users[len(list.Users)-1]; last.Email != "napa@example.com" || !last.PasswordSet {
		t.Errorf("last user = %+v, want napa with a password set", last)
	}
}

// context ADR 0002: the token is shown once and stored so that reading the database does
// not reveal it.
func TestTheSetupLinkTokenCannotBeReadAgain(t *testing.T) {
	h := newHarness(t)
	h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
	owner := h.session(ownerSessionEmail)
	created := decode[createdUser](t, h.createUser(newUser("Napa", "napa@example.com", "staff"), owner))

	list := h.listUsers(owner)

	if strings.Contains(list.Body.String(), created.SetupLink.Token) {
		t.Errorf("the list shows the token: %s", list.Body)
	}
	links := h.store.SetupLinks()
	if len(links) != 1 {
		t.Fatalf("stored links = %d, want 1", len(links))
	}
	if !bytes.Equal(links[0].TokenHash, auth.HashToken(created.SetupLink.Token)) {
		t.Errorf("stored %x, want the hash of the token", links[0].TokenHash)
	}
}

// users-and-access R9: the Role Owner is refused, with a field error on the Role, and
// no User is made.
func TestCreateUserRefusesTheRoleOwner(t *testing.T) {
	h := newHarness(t)
	h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })

	rec := h.createUser(newUser("Napa", "napa@example.com", "owner"), h.session(ownerSessionEmail))

	assertProblem(t, rec, http.StatusUnprocessableEntity, "")
	if got := decode[problem](t, rec); len(got.Errors) != 1 || got.Errors[0].Location != "body.role" {
		t.Errorf("errors = %+v, want one at body.role", got.Errors)
	}
	if _, err := h.store.GetUserByEmail(t.Context(), "napa@example.com"); err == nil {
		t.Error("a User was created")
	}
}

// users-and-access R14: a name is required, an email must be an email, and a Role must
// be one of the two.
func TestCreateUserRefusesWhatIsNotAUser(t *testing.T) {
	cases := map[string]struct {
		body     any
		location string
	}{
		"no name":         {map[string]string{"email": "napa@example.com", "role": "staff"}, "body: required property name"},
		"an empty name":   {newUser("", "napa@example.com", "staff"), "body.name"},
		"a blank name":    {newUser("   ", "napa@example.com", "staff"), "body.name"},
		"no email":        {map[string]string{"name": "Napa", "role": "staff"}, "body: required property email"},
		"not an email":    {newUser("Napa", "napa", "staff"), "body.email"},
		"no role":         {map[string]string{"name": "Napa", "email": "napa@example.com"}, "body: required property role"},
		"an unknown role": {newUser("Napa", "napa@example.com", "manager"), "body.role"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })

			rec := h.createUser(c.body, h.session(ownerSessionEmail))

			assertProblem(t, rec, http.StatusUnprocessableEntity, "")
			got := decode[problem](t, rec)
			if len(got.Errors) != 1 || !fieldError(got.Errors[0].Location, got.Errors[0].Message, c.location) {
				t.Errorf("errors = %+v, want one at %s", got.Errors, c.location)
			}
		})
	}
}

// users-and-access R3.
func TestCreateUserRefusesAnEmailInUseWhateverItsLetterCase(t *testing.T) {
	for _, taken := range []string{"somchai@example.com", "SomChai@Example.COM"} {
		t.Run(taken, func(t *testing.T) {
			h := newHarness(t)
			h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
			h.addUser("somchai@example.com")
			owner := h.session(ownerSessionEmail)

			rec := h.createUser(newUser("Another Somchai", taken, "staff"), owner)

			assertProblem(t, rec, http.StatusConflict, "email-taken")
			if len(h.store.SetupLinks()) != 0 {
				t.Error("a Setup link was issued for a User that was not created")
			}
		})
	}
}

// users-and-access R3: the new User's email is also taken for the next one, in any case.
func TestCreateUserTwiceWithTheSameEmailRefusesTheSecond(t *testing.T) {
	h := newHarness(t)
	h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
	owner := h.session(ownerSessionEmail)
	if rec := h.createUser(newUser("Napa", "napa@example.com", "staff"), owner); rec.Code != http.StatusCreated {
		t.Fatalf("first: status = %d; body %s", rec.Code, rec.Body)
	}

	rec := h.createUser(newUser("Napa again", "NAPA@example.com", "admin"), owner)

	assertProblem(t, rec, http.StatusConflict, "email-taken")
}

// users-and-access R14: a name in Thai is stored and returned unchanged. R15: so is the
// email, as it was entered.
func TestAThaiNameIsStoredAndReturnedUnchanged(t *testing.T) {
	h := newHarness(t)
	h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
	owner := h.session(ownerSessionEmail)
	const name = "สมชาย ใจดี"

	created := decode[createdUser](t, h.createUser(newUser(name, "Somchai@Example.com", "staff"), owner))

	if created.User.Name != name || created.User.Email != "Somchai@Example.com" {
		t.Errorf("created = %+v", created.User)
	}
	list := decode[struct {
		Users []userBody `json:"users"`
	}](t, h.listUsers(owner))
	last := list.Users[len(list.Users)-1]
	if last.Name != name || last.Email != "Somchai@Example.com" {
		t.Errorf("listed = %+v", last)
	}
}

// users-and-access R23, and the ticket: Admin is the Owner's equal elsewhere, but not
// here.
func TestUsersOperationsRefuseEveryoneButTheOwner(t *testing.T) {
	for _, role := range []string{"admin", "staff"} {
		t.Run(role, func(t *testing.T) {
			h := newHarness(t)
			h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
			target := h.addUser("target@example.com")
			targetSession := h.session("target@example.com")
			cookie := h.loggedInAs(role)

			list := h.listUsers(cookie)
			create := h.createUser(newUser("Napa", "napa@example.com", "staff"), cookie)
			update := h.updateUser(target.ID, inactive, cookie)
			reset := h.resetPassword(target.ID, cookie)

			assertProblem(t, list, http.StatusForbidden, "")
			assertProblem(t, create, http.StatusForbidden, "")
			assertProblem(t, update, http.StatusForbidden, "")
			assertProblem(t, reset, http.StatusForbidden, "")
			if _, err := h.store.GetUserByEmail(t.Context(), "napa@example.com"); err == nil {
				t.Error("a User was created")
			}
			if rec := h.me(targetSession); rec.Code != http.StatusOK {
				t.Errorf("the target's session: status = %d, want it untouched", rec.Code)
			}
			if rec := h.logIn("target@example.com", password); rec.Code != http.StatusOK {
				t.Errorf("the target's password: status = %d, want it untouched", rec.Code)
			}
		})
	}
}

// users-and-access R23, R26: the same 401s as every operation that needs a session.
func TestUsersOperationsNeedALiveSession(t *testing.T) {
	operations := map[string]func(*harness, ...*http.Cookie) *httptest.ResponseRecorder{
		"list users": (*harness).listUsers,
		"create user": func(h *harness, cookies ...*http.Cookie) *httptest.ResponseRecorder {
			return h.createUser(newUser("Napa", "napa@example.com", "staff"), cookies...)
		},
		"update user": func(h *harness, cookies ...*http.Cookie) *httptest.ResponseRecorder {
			return h.updateUser(1, inactive, cookies...)
		},
		"reset password": func(h *harness, cookies ...*http.Cookie) *httptest.ResponseRecorder {
			return h.resetPassword(1, cookies...)
		},
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)

			assertProblem(t, operation(h), http.StatusUnauthorized, "")
			unknown := operation(h, &http.Cookie{Name: "session", Value: "no-such-session"})
			assertProblem(t, unknown, http.StatusUnauthorized, "session-ended")
		})
	}
}

// Who is refused is decided before what they sent is looked at: a visitor or an Admin
// with a bad body hears 401 or 403, not what is wrong with the body.
func TestAccessIsCheckedBeforeTheBody(t *testing.T) {
	h := newHarness(t)
	admin := h.loggedInAs("admin")
	invalid := newUser("", "not an email", "owner")

	assertProblem(t, h.createUser(invalid), http.StatusUnauthorized, "")
	assertProblem(t, h.createUser(invalid, admin), http.StatusForbidden, "")
	assertProblem(t, h.updateUser(1, invalid), http.StatusUnauthorized, "")
	assertProblem(t, h.updateUser(1, invalid, admin), http.StatusForbidden, "")
}

func TestUsersOperationsAnswerAStoreFailureWithoutItsCause(t *testing.T) {
	h := newHarness(t)
	h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
	owner := h.session(ownerSessionEmail)
	h.store.Err = errors.New("connection to db.internal:5432 refused")

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"list users":  h.listUsers(owner),
		"create user": h.createUser(newUser("Napa", "napa@example.com", "staff"), owner),
	} {
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, want 500; body %s", name, rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), "db.internal") {
			t.Errorf("%s: body leaks the cause: %s", name, rec.Body)
		}
	}
}

// fieldError matches a field error against where, either a location like "body.name" or,
// for a field missing from the body, "body: required property name": Huma reports that
// one at the body and names the field in the message.
func fieldError(location, message, where string) bool {
	if place, property, missing := strings.Cut(where, ": required property "); missing {
		return location == place && strings.Contains(message, "property "+property+" ")
	}
	return location == where
}
