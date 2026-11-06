package storetest

import (
	"bytes"
	"context"
	"errors"
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
		if err := s.Store.CreateSession(ctx, store.CreateSessionParams{TokenHash: hash, UserID: user.ID, ExpiresAt: at}); err != nil {
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
			if err := s.Store.CreateSession(ctx, store.CreateSessionParams{TokenHash: []byte(hash), UserID: user.ID, ExpiresAt: at}); err != nil {
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

	t.Run("refuses a session or a Setup link for no User", func(t *testing.T) {
		s := newSubject(t)

		if err := s.Store.CreateSession(ctx, store.CreateSessionParams{TokenHash: []byte("h"), UserID: 404, ExpiresAt: at}); err == nil {
			t.Error("CreateSession: err = nil, want a refusal")
		}
		if err := s.Store.IssueSetupLink(ctx, store.IssueSetupLinkParams{UserID: 404, TokenHash: []byte("h"), ExpiresAt: at}); err == nil {
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
}
