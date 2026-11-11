//go:build slow

package store_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"
	"sync"
	"testing"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/testdb"
)

// users-and-access R42, R48: the real transaction exchanges a code only once.
func TestConcurrentFirstTimeCodeVerificationsIssueOnlyOneGrant(t *testing.T) {
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	u := addUser(t, pool, store.User{Name: "New", Email: "new@example.com", Role: "staff", Status: "active"})
	if _, err := s.IssuePasswordCredential(t.Context(), store.EmailCredentialParams{Email: u.Email, Kind: store.FirstTimeCode, TokenHash: []byte("code"), ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	type result struct {
		ok  bool
		err error
	}
	results := make(chan result, 12)
	start := make(chan struct{})
	var group sync.WaitGroup
	for range 12 {
		group.Go(func() {
			<-start
			ok, err := s.VerifyFirstTimeCode(context.Background(), store.VerifyFirstTimeCodeParams{Email: u.Email, CodeHash: []byte("code"), GrantHash: []byte("grant"), GrantExpiresAt: now.Add(time.Minute), At: now})
			results <- result{ok, err}
		})
	}
	close(start)
	group.Wait()
	close(results)
	winners := 0
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.ok {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("verified %d times, want one", winners)
	}
}

// users-and-access R43: separate connections cannot oversubscribe the IP allowance.
func TestConcurrentEmailRequestsCannotExceedThirtyPerIP(t *testing.T) {
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	type result struct {
		ok  bool
		err error
	}
	results := make(chan result, 35)
	start := make(chan struct{})
	var group sync.WaitGroup
	for i := range 35 {
		group.Go(func() {
			<-start
			ok, err := s.ReserveEmailRequest(context.Background(), store.EmailRequestParams{EmailHash: []byte(fmt.Sprint(i)), IPHash: []byte("shared-ip"), At: now})
			results <- result{ok, err}
		})
	}
	close(start)
	group.Wait()
	close(results)
	allowed := 0
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.ok {
			allowed++
		}
	}
	if allowed != 30 {
		t.Fatalf("allowed %d requests, want thirty", allowed)
	}
}

// R32: a completed deactivation cannot be followed by credential resurrection.
func TestCredentialIssuanceWaitsForDeactivationAndRefusesTheInactiveUser(t *testing.T) {
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	u := addUser(t, pool, store.User{Name: "New", Email: "new@example.com", Role: "staff", Status: "active"})
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(t.Context(), "UPDATE users SET status = 'inactive' WHERE id = $1", u.ID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := s.IssuePasswordCredential(context.Background(), store.EmailCredentialParams{Email: u.Email, Kind: store.FirstTimeCode, TokenHash: []byte("code"), ExpiresAt: now.Add(time.Minute)})
		result <- err
	}()
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != store.ErrNotFound {
		t.Fatalf("issuance after deactivation = %v", err)
	}
}

// R31/R48: only one concurrent recovery completion can change the password.
func TestConcurrentResetCompletionsConsumeTheLinkOnlyOnce(t *testing.T) {
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	u := addUser(t, pool, store.User{Name: "User", Email: "reset@example.com", Role: "staff", Status: "active", PasswordHash: pgtype.Text{String: "old-hash", Valid: true}})
	if _, err := s.IssuePasswordCredential(t.Context(), store.EmailCredentialParams{Email: u.Email, Kind: store.ResetLink, TokenHash: []byte("reset"), ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 8)
	start := make(chan struct{})
	var group sync.WaitGroup
	for i := range 8 {
		group.Go(func() {
			<-start
			_, err := s.SetEmailPassword(context.Background(), store.EmailPasswordParams{Kind: store.ResetLink, TokenHash: []byte("reset"), At: now, PasswordHash: fmt.Sprint(i)})
			results <- err
		})
	}
	close(start)
	group.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, store.ErrNotFound) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("reset succeeded %d times", winners)
	}
}

// R31/R48: a failure after password/session writes rolls the entire change back.
func TestResetFailureRollsBackPasswordSessionsAndCredentialConsumption(t *testing.T) {
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	u := addUser(t, pool, store.User{Name: "User", Email: "reset@example.com", Role: "staff", Status: "active", PasswordHash: pgtype.Text{String: "old-hash", Valid: true}})
	if _, err := s.CreateSession(t.Context(), store.CreateSessionParams{UserID: u.ID, PasswordHash: u.PasswordHash, TokenHash: []byte("session"), ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssuePasswordCredential(t.Context(), store.EmailCredentialParams{Email: u.Email, Kind: store.ResetLink, TokenHash: []byte("reset"), ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(t.Context(), `CREATE FUNCTION refuse_email_consumption() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected storage failure'; END $$; CREATE TRIGGER refuse_email_consumption BEFORE DELETE ON email_credentials FOR EACH ROW EXECUTE FUNCTION refuse_email_consumption()`)
	if err != nil {
		t.Fatal(err)
	}
	arg := store.EmailPasswordParams{Kind: store.ResetLink, TokenHash: []byte("reset"), At: now, PasswordHash: "new-hash"}
	if _, err := s.SetEmailPassword(t.Context(), arg); err == nil {
		t.Fatal("injected failure was ignored")
	}
	got, err := s.GetUserByEmail(t.Context(), u.Email)
	if err != nil || got.PasswordHash != u.PasswordHash {
		t.Fatalf("password was not rolled back: %+v, %v", got, err)
	}
	if _, err := s.GetSession(t.Context(), []byte("session")); err != nil {
		t.Fatal("session was not restored", err)
	}
	if _, err := s.ReadEmailPasswordUser(t.Context(), arg); err != nil {
		t.Fatal("credential consumption was not rolled back", err)
	}
}

// R31 and existing login serialization: no old-password login survives recovery.
func TestResetRevokesSessionsFromOverlappingOldPasswordLogins(t *testing.T) {
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	u := addUser(t, pool, store.User{Name: "User", Email: "reset@example.com", Role: "staff", Status: "active", PasswordHash: pgtype.Text{String: "old-hash", Valid: true}})
	if _, err := s.IssuePasswordCredential(t.Context(), store.EmailCredentialParams{Email: u.Email, Kind: store.ResetLink, TokenHash: []byte("reset"), ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	failures := make(chan error, 9)
	var group sync.WaitGroup
	for i := range 8 {
		group.Go(func() {
			<-start
			_, err := s.StartSession(context.Background(), store.StartSessionParams{
				Session: store.CreateSessionParams{UserID: u.ID, PasswordHash: u.PasswordHash, TokenHash: []byte(fmt.Sprint(i)), ExpiresAt: now.Add(time.Hour)},
				Entry:   store.AddLoginLogEntryParams{At: now, Email: u.Email, UserID: pgtype.Int8{Int64: u.ID, Valid: true}},
			})
			failures <- err
		})
	}
	group.Go(func() {
		<-start
		_, err := s.SetEmailPassword(context.Background(), store.EmailPasswordParams{Kind: store.ResetLink, TokenHash: []byte("reset"), At: now, PasswordHash: "new-hash"})
		failures <- err
	})
	close(start)
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := range 8 {
		if _, err := s.GetSession(t.Context(), []byte(fmt.Sprint(i))); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("old-password session %d survived: %v", i, err)
		}
	}
}
