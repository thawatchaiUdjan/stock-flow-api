//go:build slow

package store_test

import (
	"context"
	"fmt"
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
