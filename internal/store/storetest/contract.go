package storetest

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/api"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// Store is every port the database satisfies.
type Store interface {
	api.Store
	auth.OwnerStore
	CreateSession(ctx context.Context, arg store.CreateSessionParams) (int64, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error
}

// Subject is one implementation of Store under the contract suite. AddUser seeds a User
// the way only later features can (any Role, any status), because the port has no
// operation for it yet.
type Subject struct {
	Store   Store
	AddUser func(t *testing.T, user store.User) store.User
}

// at is a fixed instant with Postgres's microsecond precision, so it survives a round trip.
var at = time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)

func staff(email string) store.User {
	return store.User{Name: "Somchai", Email: email, Role: "staff", Status: "active"}
}

// Run is the contract of the store ports. The fake and the real store both pass it, so
// the fake can't be more permissive than Postgres (docs/agents/testing.md).
func Run(t *testing.T, newSubject func(t *testing.T) Subject) {
	ctx := context.Background()

	t.Run("finds a User by email whatever its letter case", func(t *testing.T) {
		s := newSubject(t)
		added := s.AddUser(t, staff("Somchai@Example.com"))

		got, err := s.Store.GetUserByEmail(ctx, "sOMCHAI@example.COM")

		if err != nil {
			t.Fatal(err)
		}
		if got != added {
			t.Errorf("got %+v, want %+v", got, added)
		}
		if got.Email != "Somchai@Example.com" {
			t.Errorf("email = %q, want it as it was stored", got.Email)
		}
	})

	t.Run("reports an unknown email as not found", func(t *testing.T) {
		s := newSubject(t)

		_, err := s.Store.GetUserByEmail(ctx, "nobody@example.com")

		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("reports no Owner as not found", func(t *testing.T) {
		s := newSubject(t)
		s.AddUser(t, staff("staff@example.com"))

		_, err := s.Store.GetOwner(ctx)

		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("creates an Owner who is Active with no password", func(t *testing.T) {
		s := newSubject(t)

		if err := s.Store.CreateOwner(ctx, store.CreateOwnerParams{Name: "Malee", Email: "malee@example.com"}); err != nil {
			t.Fatal(err)
		}

		got, err := s.Store.GetOwner(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "Malee" || got.Email != "malee@example.com" || got.Role != "owner" || got.Status != "active" || got.PasswordHash.Valid {
			t.Errorf("owner = %+v", got)
		}
	})

	t.Run("creating a second Owner changes nothing", func(t *testing.T) {
		s := newSubject(t)
		first := store.CreateOwnerParams{Name: "Malee", Email: "malee@example.com"}
		if err := s.Store.CreateOwner(ctx, first); err != nil {
			t.Fatal(err)
		}

		if err := s.Store.CreateOwner(ctx, store.CreateOwnerParams{Name: "Other", Email: "other@example.com"}); err != nil {
			t.Fatal(err)
		}

		got, err := s.Store.GetOwner(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.Email != first.Email {
			t.Errorf("owner email = %q, want %q", got.Email, first.Email)
		}
		if _, err := s.Store.GetUserByEmail(ctx, "other@example.com"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("second Owner's email lookup: err = %v, want ErrNotFound", err)
		}
	})

	t.Run("refuses a blank name", func(t *testing.T) {
		s := newSubject(t)

		err := s.Store.CreateOwner(ctx, store.CreateOwnerParams{Name: "  ", Email: "malee@example.com"})

		if err == nil {
			t.Error("err = nil, want a refusal")
		}
	})

	t.Run("a session reads the User's current name, email and Role, and its expiry", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("staff@example.com"))
		hash := []byte("session-hash")
		if _, err := s.Store.CreateSession(ctx, store.CreateSessionParams{TokenHash: hash, UserID: user.ID, ExpiresAt: at}); err != nil {
			t.Fatal(err)
		}

		got, err := s.Store.GetSession(ctx, hash)

		if err != nil {
			t.Fatal(err)
		}
		if got.Name != user.Name || got.Email != user.Email || got.Role != "staff" || !got.ExpiresAt.Equal(at) {
			t.Errorf("session = %+v", got)
		}
	})

	t.Run("deleting a session leaves the User's other sessions", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("staff@example.com"))
		for _, hash := range []string{"one", "two"} {
			if _, err := s.Store.CreateSession(ctx, store.CreateSessionParams{TokenHash: []byte(hash), UserID: user.ID, ExpiresAt: at}); err != nil {
				t.Fatal(err)
			}
		}

		if err := s.Store.DeleteSession(ctx, []byte("one")); err != nil {
			t.Fatal(err)
		}

		if _, err := s.Store.GetSession(ctx, []byte("one")); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("deleted session: err = %v, want ErrNotFound", err)
		}
		if _, err := s.Store.GetSession(ctx, []byte("two")); err != nil {
			t.Errorf("other session: %v", err)
		}
	})

	// users-and-access R16, R31: what the password was checked against has to still hold.
	t.Run("starts no session for no User, an Inactive User or a password that has changed", func(t *testing.T) {
		s := newSubject(t)
		hash := pgtype.Text{String: "hash", Valid: true}
		inactive := staff("inactive@example.com")
		inactive.Status, inactive.PasswordHash = "inactive", hash
		inactiveID := s.AddUser(t, inactive).ID
		changed := staff("changed@example.com")
		changed.PasswordHash = pgtype.Text{String: "newer-hash", Valid: true}
		changedID := s.AddUser(t, changed).ID
		resetID := s.AddUser(t, staff("reset@example.com")).ID

		for name, userID := range map[string]int64{"no User": 404, "Inactive": inactiveID, "changed": changedID, "reset": resetID} {
			token := []byte(name)
			started, err := s.Store.CreateSession(ctx, store.CreateSessionParams{TokenHash: token, UserID: userID, ExpiresAt: at, PasswordHash: hash})
			if err != nil || started != 0 {
				t.Errorf("%s: started = %d, err = %v, want 0 and nil", name, started, err)
			}
			if _, err := s.Store.GetSession(ctx, token); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("%s: err = %v, want no session", name, err)
			}
		}
	})

	t.Run("starts a session for an Active User with the password that was checked", func(t *testing.T) {
		s := newSubject(t)
		hash := pgtype.Text{String: "hash", Valid: true}
		user := staff("staff@example.com")
		user.PasswordHash = hash
		user = s.AddUser(t, user)

		started, err := s.Store.CreateSession(ctx, store.CreateSessionParams{TokenHash: []byte("h"), UserID: user.ID, ExpiresAt: at, PasswordHash: hash})

		if err != nil || started != 1 {
			t.Errorf("started = %d, err = %v, want 1 and nil", started, err)
		}
	})

	t.Run("refuses a Setup link for no User", func(t *testing.T) {
		s := newSubject(t)

		if err := s.Store.IssueSetupLink(ctx, store.IssueSetupLinkParams{UserID: 404, TokenHash: []byte("h"), ExpiresAt: at}); err == nil {
			t.Error("IssueSetupLink: err = nil, want a refusal")
		}
	})

	t.Run("refuses a token hash already in use", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("staff@example.com"))
		other := s.AddUser(t, staff("other@example.com"))
		session := store.CreateSessionParams{TokenHash: []byte("h"), UserID: user.ID, ExpiresAt: at}
		if _, err := s.Store.CreateSession(ctx, session); err != nil {
			t.Fatal(err)
		}
		link := store.IssueSetupLinkParams{UserID: user.ID, TokenHash: []byte("h"), ExpiresAt: at}
		if err := s.Store.IssueSetupLink(ctx, link); err != nil {
			t.Fatal(err)
		}

		if _, err := s.Store.CreateSession(ctx, session); err == nil {
			t.Error("CreateSession: err = nil, want a refusal")
		}
		link.UserID = other.ID
		if err := s.Store.IssueSetupLink(ctx, link); err == nil {
			t.Error("IssueSetupLink: err = nil, want a refusal")
		}
	})

	issue := func(t *testing.T, s Subject, user store.User, hash string) {
		t.Helper()
		err := s.Store.IssueSetupLink(ctx, store.IssueSetupLinkParams{UserID: user.ID, TokenHash: []byte(hash), ExpiresAt: at})
		if err != nil {
			t.Fatal(err)
		}
	}
	setPassword := func(t *testing.T, s Subject, hash string, when time.Time) int64 {
		t.Helper()
		used, err := s.Store.SetPassword(ctx, store.SetPasswordParams{PasswordHash: "new-hash", TokenHash: []byte(hash), At: when})
		if err != nil {
			t.Fatal(err)
		}
		return used
	}

	t.Run("a Setup link names its person until it expires", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("staff@example.com"))
		issue(t, s, user, "link")

		got, err := s.Store.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: []byte("link"), At: at.Add(-time.Second)})
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != user.Name || got.Email != user.Email {
			t.Errorf("got %+v", got)
		}

		_, err = s.Store.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: []byte("link"), At: at})
		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("at its expiry: err = %v, want ErrNotFound", err)
		}
	})

	t.Run("issuing a Setup link replaces the User's earlier one", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("staff@example.com"))
		issue(t, s, user, "first")
		issue(t, s, user, "second")

		_, err := s.Store.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: []byte("first"), At: at.Add(-time.Second)})
		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("earlier link: err = %v, want ErrNotFound", err)
		}
		if used := setPassword(t, s, "first", at.Add(-time.Second)); used != 0 {
			t.Errorf("earlier link set a password")
		}
		if used := setPassword(t, s, "second", at.Add(-time.Second)); used != 1 {
			t.Errorf("newest link: used = %d, want 1", used)
		}
	})

	t.Run("a Setup link sets its User's password, once", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("staff@example.com"))
		other := s.AddUser(t, staff("other@example.com"))
		issue(t, s, user, "link")

		first := setPassword(t, s, "link", at.Add(-time.Second))
		second := setPassword(t, s, "link", at.Add(-time.Second))

		if first != 1 || second != 0 {
			t.Errorf("used = %d then %d, want 1 then 0", first, second)
		}
		got, err := s.Store.GetUserByEmail(ctx, user.Email)
		if err != nil {
			t.Fatal(err)
		}
		if want := (pgtype.Text{String: "new-hash", Valid: true}); got.PasswordHash != want {
			t.Errorf("password hash = %+v, want %+v", got.PasswordHash, want)
		}
		untouched, err := s.Store.GetUserByEmail(ctx, other.Email)
		if err != nil {
			t.Fatal(err)
		}
		if untouched.PasswordHash.Valid {
			t.Errorf("another User's password changed: %+v", untouched.PasswordHash)
		}
	})

	t.Run("an expired or unknown Setup link sets no password", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("staff@example.com"))
		issue(t, s, user, "link")

		if used := setPassword(t, s, "link", at); used != 0 {
			t.Errorf("at its expiry: used = %d, want 0", used)
		}
		if used := setPassword(t, s, "unknown", at.Add(-time.Second)); used != 0 {
			t.Errorf("unknown link: used = %d, want 0", used)
		}
		got, err := s.Store.GetUserByEmail(ctx, user.Email)
		if err != nil {
			t.Fatal(err)
		}
		if got.PasswordHash.Valid {
			t.Errorf("password hash = %+v, want none", got.PasswordHash)
		}
	})

	owner := func(email string) store.User {
		return store.User{Name: "Malee", Email: email, Role: "owner", Status: "active"}
	}
	session := func(t *testing.T, s Subject, user store.User, hash string) {
		t.Helper()
		started, err := s.Store.CreateSession(ctx, store.CreateSessionParams{
			TokenHash: []byte(hash), UserID: user.ID, ExpiresAt: at, PasswordHash: user.PasswordHash,
		})
		if err != nil || started != 1 {
			t.Fatalf("started = %d, err = %v", started, err)
		}
	}
	live := func(s Subject, hash string) bool {
		_, err := s.Store.GetSession(ctx, []byte(hash))
		return err == nil
	}

	// context ADR 0003, users-and-access R34.
	t.Run("setting the Owner's password ends all the Owner's sessions and no one else's", func(t *testing.T) {
		s := newSubject(t)
		malee := s.AddUser(t, owner("malee@example.com"))
		somchai := s.AddUser(t, staff("staff@example.com"))
		session(t, s, malee, "laptop")
		session(t, s, malee, "phone")
		session(t, s, somchai, "staff-laptop")
		issue(t, s, malee, "link")

		if used := setPassword(t, s, "link", at.Add(-time.Second)); used != 1 {
			t.Fatalf("used = %d, want 1", used)
		}

		if live(s, "laptop") || live(s, "phone") {
			t.Error("an Owner's session is still live")
		}
		if !live(s, "staff-laptop") {
			t.Error("another User's session ended")
		}
	})

	t.Run("setting a password the Owner has no session for still works", func(t *testing.T) {
		s := newSubject(t)
		malee := s.AddUser(t, owner("malee@example.com"))
		issue(t, s, malee, "link")

		if used := setPassword(t, s, "link", at.Add(-time.Second)); used != 1 {
			t.Errorf("used = %d, want 1", used)
		}
	})

	t.Run("setting a non-Owner's password ends no session", func(t *testing.T) {
		s := newSubject(t)
		malee := s.AddUser(t, owner("malee@example.com"))
		somchai := s.AddUser(t, staff("staff@example.com"))
		session(t, s, malee, "owner-laptop")
		session(t, s, somchai, "staff-laptop")
		issue(t, s, somchai, "link")

		if used := setPassword(t, s, "link", at.Add(-time.Second)); used != 1 {
			t.Fatalf("used = %d, want 1", used)
		}

		if !live(s, "staff-laptop") || !live(s, "owner-laptop") {
			t.Error("a session ended")
		}
	})

	t.Run("an expired or unknown Setup link ends no session", func(t *testing.T) {
		s := newSubject(t)
		malee := s.AddUser(t, owner("malee@example.com"))
		session(t, s, malee, "laptop")
		issue(t, s, malee, "link")

		if used := setPassword(t, s, "link", at); used != 0 {
			t.Errorf("at its expiry: used = %d, want 0", used)
		}
		if used := setPassword(t, s, "unknown", at.Add(-time.Second)); used != 0 {
			t.Errorf("unknown link: used = %d, want 0", used)
		}

		if !live(s, "laptop") {
			t.Error("the Owner's session ended")
		}
	})

	t.Run("a token hash round-trips as bytes, not text", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("staff@example.com"))
		hash := bytes.Repeat([]byte{0x00, 0xff}, 16)

		err := s.Store.IssueSetupLink(ctx, store.IssueSetupLinkParams{UserID: user.ID, TokenHash: hash, ExpiresAt: at})
		if err != nil {
			t.Fatal(err)
		}

		if _, err := s.Store.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: hash, At: at.Add(-time.Second)}); err != nil {
			t.Errorf("lookup by the same bytes: %v", err)
		}
	})

	t.Run("creates an Active User with no password, and their Setup link", func(t *testing.T) {
		s := newSubject(t)

		created, err := s.Store.CreateUser(ctx, store.CreateUserParams{
			Name: "สมชาย", Email: "Somchai@Example.com", Role: "admin", TokenHash: []byte("link"), ExpiresAt: at,
		})

		if err != nil {
			t.Fatal(err)
		}
		want := store.CreateUserRow{ID: created.ID, Name: "สมชาย", Email: "Somchai@Example.com", Role: "admin", Status: "active"}
		if created != want || created.ID == 0 {
			t.Errorf("created = %+v, want %+v", created, want)
		}
		got, err := s.Store.GetUserByEmail(ctx, "somchai@example.com")
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != created.ID || got.PasswordHash.Valid {
			t.Errorf("stored = %+v", got)
		}
		link, err := s.Store.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: []byte("link"), At: at.Add(-time.Second)})
		if err != nil {
			t.Fatalf("the new User's Setup link: %v", err)
		}
		if link.Name != "สมชาย" || link.Email != "Somchai@Example.com" {
			t.Errorf("link is for %+v", link)
		}
	})

	t.Run("an email in use creates no User and no Setup link, whatever its letter case", func(t *testing.T) {
		s := newSubject(t)
		s.AddUser(t, staff("somchai@example.com"))

		_, err := s.Store.CreateUser(ctx, store.CreateUserParams{
			Name: "Another", Email: "SOMCHAI@example.com", Role: "staff", TokenHash: []byte("link"), ExpiresAt: at,
		})

		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
		_, err = s.Store.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: []byte("link"), At: at.Add(-time.Second)})
		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Setup link: err = %v, want ErrNotFound", err)
		}
		users, err := s.Store.ListUsers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 {
			t.Errorf("users = %+v, want only the first", users)
		}
	})

	t.Run("refuses a blank name, an unknown Role and a second Owner", func(t *testing.T) {
		s := newSubject(t)
		s.AddUser(t, store.User{Name: "Malee", Email: "malee@example.com", Role: "owner", Status: "active"})

		for name, arg := range map[string]store.CreateUserParams{
			"blank name":   {Name: " ", Email: "a@example.com", Role: "staff"},
			"unknown Role": {Name: "A", Email: "b@example.com", Role: "manager"},
			"second Owner": {Name: "A", Email: "c@example.com", Role: "owner"},
		} {
			arg.TokenHash, arg.ExpiresAt = []byte(name), at
			if _, err := s.Store.CreateUser(ctx, arg); err == nil || errors.Is(err, store.ErrNotFound) {
				t.Errorf("%s: err = %v, want a refusal", name, err)
			}
		}
	})

	t.Run("a blank name is refused even when the email is in use", func(t *testing.T) {
		s := newSubject(t)
		s.AddUser(t, staff("somchai@example.com"))

		_, err := s.Store.CreateUser(ctx, store.CreateUserParams{
			Name: " ", Email: "somchai@example.com", Role: "staff", TokenHash: []byte("link"), ExpiresAt: at,
		})

		if err == nil || errors.Is(err, store.ErrNotFound) {
			t.Errorf("err = %v, want a refusal that is not ErrNotFound", err)
		}
	})

	t.Run("lists every User in the order they were made, with whether a password is set", func(t *testing.T) {
		s := newSubject(t)
		first := s.AddUser(t, store.User{
			Name: "Malee", Email: "malee@example.com", Role: "owner", Status: "active",
			PasswordHash: pgtype.Text{String: "hash", Valid: true},
		})
		second := s.AddUser(t, store.User{Name: "Napa", Email: "napa@example.com", Role: "admin", Status: "inactive"})

		got, err := s.Store.ListUsers(ctx)

		if err != nil {
			t.Fatal(err)
		}
		want := []store.ListUsersRow{
			{ID: first.ID, Name: "Malee", Email: "malee@example.com", Role: "owner", Status: "active", PasswordSet: true},
			{ID: second.ID, Name: "Napa", Email: "napa@example.com", Role: "admin", Status: "inactive", PasswordSet: false},
		}
		if !slices.Equal(got, want) {
			t.Errorf("users = %+v, want %+v", got, want)
		}
	})

	t.Run("lists no User as an empty list", func(t *testing.T) {
		s := newSubject(t)

		got, err := s.Store.ListUsers(ctx)

		if err != nil || len(got) != 0 {
			t.Errorf("users = %+v, err = %v", got, err)
		}
	})

	withPassword := func(user store.User) store.User {
		user.PasswordHash = pgtype.Text{String: "hash", Valid: true}
		return user
	}
	linkWorks := func(s Subject, hash string) bool {
		_, err := s.Store.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: []byte(hash), At: at.Add(-time.Second)})
		return err == nil
	}

	t.Run("updating a User changes what is named and nothing else", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, withPassword(staff("Somchai@Example.com")))

		name, err := s.Store.UpdateUser(ctx, store.ChangeUserParams{ID: user.ID, Name: "สมชาย"})
		if err != nil {
			t.Fatal(err)
		}
		role, err := s.Store.UpdateUser(ctx, store.ChangeUserParams{ID: user.ID, Role: "admin"})
		if err != nil {
			t.Fatal(err)
		}
		status, err := s.Store.UpdateUser(ctx, store.ChangeUserParams{ID: user.ID, Status: "inactive"})
		if err != nil {
			t.Fatal(err)
		}

		row := store.ChangeUserRow{ID: user.ID, Name: "สมชาย", Email: "Somchai@Example.com", Role: "staff", Status: "active", PasswordSet: true}
		if name != row {
			t.Errorf("after the name: %+v, want %+v", name, row)
		}
		row.Role = "admin"
		if role != row {
			t.Errorf("after the Role: %+v, want %+v", role, row)
		}
		row.Status = "inactive"
		if status != row {
			t.Errorf("after the status: %+v, want %+v", status, row)
		}
		stored, err := s.Store.GetUserByEmail(ctx, user.Email)
		if err != nil {
			t.Fatal(err)
		}
		if stored.PasswordHash != user.PasswordHash {
			t.Errorf("password hash = %+v, want it kept", stored.PasswordHash)
		}
	})

	t.Run("updating a User with nothing named changes nothing", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("staff@example.com"))

		got, err := s.Store.UpdateUser(ctx, store.ChangeUserParams{ID: user.ID})

		want := store.ChangeUserRow{ID: user.ID, Name: user.Name, Email: user.Email, Role: "staff", Status: "active"}
		if err != nil || got != want {
			t.Errorf("got %+v, err = %v, want %+v", got, err, want)
		}
	})

	t.Run("updating or resetting no User is not found", func(t *testing.T) {
		s := newSubject(t)

		if _, err := s.Store.UpdateUser(ctx, store.ChangeUserParams{ID: 404, Name: "Nobody"}); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("update: err = %v, want ErrNotFound", err)
		}
		err := s.Store.ResetPassword(ctx, store.IssueSetupLinkParams{UserID: 404, TokenHash: []byte("h"), ExpiresAt: at})
		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("reset: err = %v, want ErrNotFound", err)
		}
	})

	// users-and-access R18, R34.
	t.Run("the Owner cannot be updated or reset, and stays as they are", func(t *testing.T) {
		s := newSubject(t)
		malee := s.AddUser(t, withPassword(owner("malee@example.com")))
		session(t, s, malee, "laptop")

		_, updateErr := s.Store.UpdateUser(ctx, store.ChangeUserParams{ID: malee.ID, Role: "staff", Status: "inactive"})
		resetErr := s.Store.ResetPassword(ctx, store.IssueSetupLinkParams{UserID: malee.ID, TokenHash: []byte("h"), ExpiresAt: at})

		if !errors.Is(updateErr, store.ErrTargetIsOwner) || !errors.Is(resetErr, store.ErrTargetIsOwner) {
			t.Errorf("update: %v, reset: %v, want ErrTargetIsOwner", updateErr, resetErr)
		}
		got, err := s.Store.GetOwner(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got != malee {
			t.Errorf("owner = %+v, want %+v", got, malee)
		}
		if !live(s, "laptop") || linkWorks(s, "h") {
			t.Errorf("session live = %v, link works = %v, want true and false", live(s, "laptop"), linkWorks(s, "h"))
		}
	})

	// The schema's own refusals: nothing above the store may rely on them, but a fake
	// that allowed these would hide a handler that stopped checking.
	t.Run("refuses a blank name, an unknown Role or status and a second Owner", func(t *testing.T) {
		s := newSubject(t)
		s.AddUser(t, owner("malee@example.com"))
		user := s.AddUser(t, staff("staff@example.com"))

		for name, arg := range map[string]store.ChangeUserParams{
			"blank name":     {ID: user.ID, Name: " "},
			"unknown Role":   {ID: user.ID, Role: "manager"},
			"unknown status": {ID: user.ID, Status: "deleted"},
			"second Owner":   {ID: user.ID, Role: "owner"},
		} {
			if _, err := s.Store.UpdateUser(ctx, arg); err == nil {
				t.Errorf("%s: err = nil, want a refusal", name)
			}
		}
		got, err := s.Store.GetUserByEmail(ctx, user.Email)
		if err != nil {
			t.Fatal(err)
		}
		if got != user {
			t.Errorf("user = %+v, want it unchanged: %+v", got, user)
		}
	})

	// users-and-access R16, R32.
	t.Run("setting a User Inactive ends their sessions and Setup link and no one else's", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("staff@example.com"))
		other := s.AddUser(t, staff("other@example.com"))
		session(t, s, user, "laptop")
		session(t, s, user, "phone")
		session(t, s, other, "other-laptop")
		issue(t, s, user, "link")
		issue(t, s, other, "other-link")

		if _, err := s.Store.UpdateUser(ctx, store.ChangeUserParams{ID: user.ID, Status: "inactive"}); err != nil {
			t.Fatal(err)
		}

		if live(s, "laptop") || live(s, "phone") || linkWorks(s, "link") {
			t.Error("a session or the Setup link is still live")
		}
		if !live(s, "other-laptop") || !linkWorks(s, "other-link") {
			t.Error("another User's session or Setup link ended")
		}
	})

	t.Run("a change that leaves a User Active ends nothing", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("staff@example.com"))
		session(t, s, user, "laptop")
		issue(t, s, user, "link")

		_, err := s.Store.UpdateUser(ctx, store.ChangeUserParams{ID: user.ID, Name: "Napa", Role: "admin", Status: "active"})
		if err != nil {
			t.Fatal(err)
		}

		if !live(s, "laptop") || !linkWorks(s, "link") {
			t.Error("a session or the Setup link ended")
		}
	})

	// users-and-access R29, R31.
	t.Run("resetting a User ends their password and sessions and replaces their Setup link", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, withPassword(staff("staff@example.com")))
		other := s.AddUser(t, withPassword(staff("other@example.com")))
		session(t, s, user, "laptop")
		session(t, s, other, "other-laptop")
		issue(t, s, user, "first")
		issue(t, s, other, "other-link")

		err := s.Store.ResetPassword(ctx, store.IssueSetupLinkParams{UserID: user.ID, TokenHash: []byte("second"), ExpiresAt: at})
		if err != nil {
			t.Fatal(err)
		}

		got, err := s.Store.GetUserByEmail(ctx, user.Email)
		if err != nil {
			t.Fatal(err)
		}
		if got.PasswordHash.Valid {
			t.Errorf("password hash = %+v, want none", got.PasswordHash)
		}
		if live(s, "laptop") || linkWorks(s, "first") {
			t.Error("the session or the earlier Setup link is still live")
		}
		if !linkWorks(s, "second") {
			t.Error("the new Setup link does not work")
		}
		untouched, err := s.Store.GetUserByEmail(ctx, other.Email)
		if err != nil {
			t.Fatal(err)
		}
		if untouched != other || !live(s, "other-laptop") || !linkWorks(s, "other-link") {
			t.Error("another User changed")
		}
	})

	// users-and-access R32.
	t.Run("resetting an Inactive User changes nothing", func(t *testing.T) {
		s := newSubject(t)
		user := withPassword(staff("staff@example.com"))
		user.Status = "inactive"
		user = s.AddUser(t, user)

		err := s.Store.ResetPassword(ctx, store.IssueSetupLinkParams{UserID: user.ID, TokenHash: []byte("link"), ExpiresAt: at})

		if !errors.Is(err, store.ErrUserInactive) {
			t.Errorf("err = %v, want ErrUserInactive", err)
		}
		got, err := s.Store.GetUserByEmail(ctx, user.Email)
		if err != nil {
			t.Fatal(err)
		}
		if got != user || linkWorks(s, "link") {
			t.Errorf("user = %+v, link works = %v, want %+v and false", got, linkWorks(s, "link"), user)
		}
	})

	t.Run("a reset whose Setup link cannot be stored changes nothing", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, withPassword(staff("staff@example.com")))
		other := s.AddUser(t, staff("other@example.com"))
		session(t, s, user, "laptop")
		issue(t, s, other, "taken")

		err := s.Store.ResetPassword(ctx, store.IssueSetupLinkParams{UserID: user.ID, TokenHash: []byte("taken"), ExpiresAt: at})

		if err == nil {
			t.Fatal("err = nil, want a refusal")
		}
		got, err := s.Store.GetUserByEmail(ctx, user.Email)
		if err != nil {
			t.Fatal(err)
		}
		if got != user || !live(s, "laptop") {
			t.Errorf("user = %+v, session live = %v, want %+v and true", got, live(s, "laptop"), user)
		}
	})

	runLoginLog(t, newSubject)
}
