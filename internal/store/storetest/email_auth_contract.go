package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// runEmailAuth keeps the new ports honest against the same real/fake subjects.
func runEmailAuth(t *testing.T, newSubject func(*testing.T) Subject) {
	t.Helper()
	ctx := context.Background()
	issue := func(t *testing.T, s Subject, address, kind, digest string) {
		t.Helper()
		_, err := s.Store.IssuePasswordCredential(ctx, store.EmailCredentialParams{
			Email: address, Kind: kind, TokenHash: []byte(digest), ExpiresAt: at.Add(10 * time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	verify := func(s Subject, address, digest string, when time.Time) (bool, error) {
		return s.Store.VerifyFirstTimeCode(ctx, store.VerifyFirstTimeCodeParams{
			Email: address, CodeHash: []byte(digest), GrantHash: []byte("grant-digest"), GrantExpiresAt: when.Add(10 * time.Minute), At: when,
		})
	}

	t.Run("email User creation preserves database constraints", func(t *testing.T) {
		s := newSubject(t)
		for _, arg := range []store.CreateEmailUserParams{
			{Name: " ", Email: "blank@example.com", Role: "staff"},
			{Name: "New", Email: "role@example.com", Role: "invalid"},
		} {
			if _, err := s.Store.CreateEmailUser(ctx, arg); err == nil {
				t.Fatalf("accepted invalid User: %+v", arg)
			}
		}
		first, err := s.Store.CreateEmailUser(ctx, store.CreateEmailUserParams{Name: "New", Email: "New@example.com", Role: "staff"})
		if err != nil || first.PasswordHash.Valid {
			t.Fatalf("create = %+v, %v", first, err)
		}
		if _, err := s.Store.CreateEmailUser(ctx, store.CreateEmailUserParams{Name: "Duplicate", Email: "NEW@example.com", Role: "admin"}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("duplicate = %v", err)
		}
	})

	t.Run("email credentials cannot share a digest across Users", func(t *testing.T) {
		s := newSubject(t)
		first := s.AddUser(t, staff("first@example.com"))
		second := s.AddUser(t, staff("second@example.com"))
		issue(t, s, first.Email, store.FirstTimeCode, "same-digest")
		_, err := s.Store.IssuePasswordCredential(ctx, store.EmailCredentialParams{Email: second.Email, Kind: store.FirstTimeCode, TokenHash: []byte("same-digest"), ExpiresAt: at.Add(time.Hour)})
		if err == nil {
			t.Fatal("duplicate digest accepted")
		}
		if ok, err := verify(s, first.Email, "same-digest", at); err != nil || !ok {
			t.Fatalf("failed issuance changed existing code: %v, %v", ok, err)
		}
	})

	t.Run("email credential purposes cannot be substituted", func(t *testing.T) {
		s := newSubject(t)
		u := s.AddUser(t, staff("new@example.com"))
		issue(t, s, u.Email, store.FirstTimeCode, "code-digest")
		_, err := s.Store.ReadEmailPasswordUser(ctx, store.EmailPasswordParams{Kind: store.FirstTimeCode, TokenHash: []byte("code-digest"), At: at})
		if !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("code accepted as password grant: %v", err)
		}
		ok, err := verify(s, "NEW@example.com", "code-digest", at)
		if err != nil || !ok {
			t.Fatalf("verify = %v, %v", ok, err)
		}
		if again, err := verify(s, u.Email, "code-digest", at); err != nil || again {
			t.Fatalf("replayed code = %v, %v", again, err)
		}
		arg := store.EmailPasswordParams{Kind: store.SetupGrant, TokenHash: []byte("grant-digest"), At: at, PasswordHash: "new-password-hash"}
		if _, err := s.Store.ReadEmailPasswordUser(ctx, store.EmailPasswordParams{Kind: store.ResetLink, TokenHash: arg.TokenHash, At: at}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("grant accepted as Reset link: %v", err)
		}
		if _, err := s.Store.SetEmailPassword(ctx, arg); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Store.SetEmailPassword(ctx, arg); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("replayed grant = %v", err)
		}
		got, err := s.Store.GetUserByEmail(ctx, u.Email)
		if err != nil || !got.PasswordHash.Valid || got.PasswordHash.String != "new-password-hash" {
			t.Fatalf("password = %+v, %v", got.PasswordHash, err)
		}
	})

	t.Run("email credential failures survive replacement and block until exact expiry", func(t *testing.T) {
		s := newSubject(t)
		u := s.AddUser(t, staff("new@example.com"))
		issue(t, s, u.Email, store.FirstTimeCode, "old-code")
		for range 4 {
			if ok, err := verify(s, u.Email, "wrong", at); err != nil || ok {
				t.Fatalf("wrong = %v, %v", ok, err)
			}
		}
		issue(t, s, u.Email, store.FirstTimeCode, "new-code")
		if ok, err := verify(s, u.Email, "wrong", at); err != nil || ok {
			t.Fatalf("fifth = %v, %v", ok, err)
		}
		if ok, err := verify(s, u.Email, "new-code", at.Add(10*time.Minute-time.Microsecond)); err != nil || ok {
			t.Fatalf("blocked = %v, %v", ok, err)
		}
		// Replace the expired code without clearing the block/allowance.
		_, err := s.Store.IssuePasswordCredential(ctx, store.EmailCredentialParams{Email: u.Email, Kind: store.FirstTimeCode, TokenHash: []byte("fresh-code"), ExpiresAt: at.Add(20 * time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		if ok, err := verify(s, u.Email, "fresh-code", at.Add(10*time.Minute)); err != nil || !ok {
			t.Fatalf("block expiry = %v, %v", ok, err)
		}
	})

	t.Run("legacy password completion cannot revive an email grant after another reset", func(t *testing.T) {
		s := newSubject(t)
		u := s.AddUser(t, staff("new@example.com"))
		issue(t, s, u.Email, store.FirstTimeCode, "code")
		if ok, err := verify(s, u.Email, "code", at); err != nil || !ok {
			t.Fatalf("verify = %v, %v", ok, err)
		}
		if err := s.Store.IssueSetupLink(ctx, store.IssueSetupLinkParams{UserID: u.ID, TokenHash: []byte("legacy"), ExpiresAt: at.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Store.SetPassword(ctx, store.SetPasswordParams{TokenHash: []byte("legacy"), PasswordHash: "legacy-password", At: at}); err != nil {
			t.Fatal(err)
		}
		if err := s.Store.ResetPassword(ctx, store.IssueSetupLinkParams{UserID: u.ID, TokenHash: []byte("legacy-reset"), ExpiresAt: at.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
		_, err := s.Store.ReadEmailPasswordUser(ctx, store.EmailPasswordParams{Kind: store.SetupGrant, TokenHash: []byte("grant-digest"), At: at})
		if !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("grant revived: %v", err)
		}
	})

	t.Run("email request limits persist between calls and expire at exact boundaries", func(t *testing.T) {
		s := newSubject(t)
		request := store.EmailRequestParams{EmailHash: []byte("email"), IPHash: []byte("ip"), At: at}
		for i := range 5 {
			request.At = at.Add(time.Duration(i) * time.Minute)
			if ok, err := s.Store.ReserveEmailRequest(ctx, request); err != nil || !ok {
				t.Fatalf("request %d = %v, %v", i, ok, err)
			}
			if ok, err := s.Store.ReserveEmailRequest(ctx, request); err != nil || ok {
				t.Fatalf("cooldown = %v, %v", ok, err)
			}
		}
		request.At = at.Add(59 * time.Minute)
		if ok, err := s.Store.ReserveEmailRequest(ctx, request); err != nil || ok {
			t.Fatalf("hour limit = %v, %v", ok, err)
		}
		request.At = at.Add(time.Hour)
		if ok, err := s.Store.ReserveEmailRequest(ctx, request); err != nil || !ok {
			t.Fatalf("hour expiry = %v, %v", ok, err)
		}
	})
}
