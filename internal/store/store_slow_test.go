//go:build slow

package store_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store/storetest"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/testdb"
)

func TestMain(m *testing.M) { os.Exit(testdb.Main(m)) }

func open(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	pool, err := store.Open(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func addUser(t *testing.T, pool *pgxpool.Pool, user store.User) store.User {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (name, email, role, status, password_hash) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		user.Name, user.Email, user.Role, user.Status, user.PasswordHash,
	).Scan(&user.ID)
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func assertViolates(t *testing.T, err error, constraint string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != constraint {
		t.Errorf("err = %v, want a violation of %s", err, constraint)
	}
}

// The fake passes the same suite, so it can't promise more than Postgres does.
func TestRealStoreKeepsTheStoreContract(t *testing.T) {
	storetest.Run(t, func(t *testing.T) storetest.Subject {
		pool := open(t, testdb.URL(t))
		return storetest.Subject{
			Store:   store.New(pool),
			AddUser: func(t *testing.T, user store.User) store.User { return addUser(t, pool, user) },
		}
	})
}

// users-and-access R3.
func TestAnEmailIsUniqueWhateverItsLetterCase(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	insert := `INSERT INTO users (name, email, role) VALUES ('Somchai', $1, 'staff')`
	if _, err := pool.Exec(ctx, insert, "somchai@example.com"); err != nil {
		t.Fatal(err)
	}

	_, err := pool.Exec(ctx, insert, "SomChai@Example.COM")

	assertViolates(t, err, "users_email_key")
}

// users-and-access R9: the schema refuses, whatever the code does.
func TestTheSchemaRefusesASecondOwner(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	insert := `INSERT INTO users (name, email, role) VALUES ('Malee', $1, 'owner')`
	if _, err := pool.Exec(ctx, insert, "malee@example.com"); err != nil {
		t.Fatal(err)
	}

	_, err := pool.Exec(ctx, insert, "other@example.com")

	assertViolates(t, err, "users_one_owner_key")
}

// users-and-access R8, R14.
func TestTheSchemaRefusesAnUnknownRoleOrStatusOrBlankName(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	insert := `INSERT INTO users (name, email, role, status) VALUES ($1, 'somchai@example.com', $2, $3)`

	_, err := pool.Exec(ctx, insert, "Somchai", "manager", "active")
	assertViolates(t, err, "users_role_check")

	_, err = pool.Exec(ctx, insert, "Somchai", "staff", "disabled")
	assertViolates(t, err, "users_status_check")

	_, err = pool.Exec(ctx, insert, "  ", "staff", "active")
	assertViolates(t, err, "users_name_check")
}

// racingInstall holds every install at its first look for the Owner until all of them have
// looked, so they all find none and all try to create one: the race, every time.
type racingInstall struct {
	*store.Queries
	looked *sync.WaitGroup
	once   sync.Once
}

func (r *racingInstall) GetOwner(ctx context.Context) (store.User, error) {
	owner, err := r.Queries.GetOwner(ctx)
	r.once.Do(func() {
		r.looked.Done()
		r.looked.Wait()
	})
	return owner, err
}

// users-and-access R9: a second Owner can't exist even under concurrent installs. Every
// run succeeds, and exactly one Setup link is left.
func TestConcurrentInstallsLeaveOneOwner(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	now := time.Now()
	const installs = 4 // the pool's MaxConns: each install holds a connection at the barrier
	var looked sync.WaitGroup
	looked.Add(installs)

	links := make([]auth.SetupLink, installs)
	errs := atTheSameTime(installs, func(i int) error {
		return store.InTx(ctx, pool, func(q *store.Queries) (err error) {
			email := fmt.Sprintf("owner%d@example.com", i)
			links[i], err = auth.IssueOwnerSetupLink(ctx, &racingInstall{Queries: q, looked: &looked}, now, "Malee", email)
			return err
		})
	})

	for i, err := range errs {
		if err != nil {
			t.Errorf("install %d: %v", i, err)
		}
	}
	var owners, setupLinks int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE role = 'owner'`).Scan(&owners); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM setup_links`).Scan(&setupLinks); err != nil {
		t.Fatal(err)
	}
	if owners != 1 || setupLinks != 1 {
		t.Errorf("owners = %d, setup links = %d, want 1 and 1", owners, setupLinks)
	}
	emails := map[string]bool{}
	for _, link := range links {
		emails[link.Email] = true
	}
	if len(emails) != 1 {
		t.Errorf("links name %d Owners, want 1: %v", len(emails), emails)
	}
}

// users-and-access R27: a Setup link opened twice at once works once.
func TestASetupLinkUsedTwiceAtOnceWorksOnce(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	now := time.Now()
	q := store.New(pool)
	link, err := auth.IssueOwnerSetupLink(ctx, q, now, "Malee", "malee@example.com")
	if err != nil {
		t.Fatal(err)
	}
	const attempts = 8
	warm(t, pool)

	used := make([]int64, attempts)
	errs := atTheSameTime(attempts, func(i int) (err error) {
		used[i], err = q.SetPassword(ctx, store.SetPasswordParams{
			PasswordHash: fmt.Sprintf("hash-%d", i),
			TokenHash:    auth.HashToken(link.Token),
			At:           now,
		})
		return err
	})

	var total int64
	for i := range attempts {
		if errs[i] != nil {
			t.Errorf("attempt %d: %v", i, errs[i])
		}
		total += used[i]
	}
	if total != 1 {
		t.Errorf("the link worked %d times, want 1", total)
	}
}

// context ADR 0003: the Owner's sessions end in the same transaction that uses up the
// Setup link, so a password that can't be set ends no session and keeps the link.
func TestAFailedPasswordSetEndsNoSessionAndKeepsTheLink(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	q := store.New(pool)
	now := time.Now().Truncate(time.Microsecond)
	link, err := auth.IssueOwnerSetupLink(ctx, q, now, "Malee", "malee@example.com")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := q.GetUserByEmail(ctx, "malee@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CreateSession(ctx, store.CreateSessionParams{TokenHash: []byte("laptop"), UserID: owner.ID, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	// The password update fails after the link is deleted and the sessions are marked
	// for deletion, which is when a missing transaction would show.
	if _, err := pool.Exec(ctx, `ALTER TABLE users ADD CONSTRAINT users_no_boom CHECK (password_hash IS DISTINCT FROM 'boom')`); err != nil {
		t.Fatal(err)
	}

	_, err = q.SetPassword(ctx, store.SetPasswordParams{PasswordHash: "boom", TokenHash: auth.HashToken(link.Token), At: now})

	assertViolates(t, err, "users_no_boom")
	if _, err := q.GetSession(ctx, []byte("laptop")); err != nil {
		t.Errorf("the Owner's session: %v, want it kept", err)
	}
	if _, err := q.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: auth.HashToken(link.Token), At: now}); err != nil {
		t.Errorf("the Setup link: %v, want it kept", err)
	}
}

// warm opens the pool's connections, so concurrent queries overlap instead of each waiting
// for a connection to be made.
func warm(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var conns []*pgxpool.Conn
	for range pool.Config().MaxConns {
		conn, err := pool.Acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, conn)
	}
	for _, conn := range conns {
		conn.Release()
	}
}

// atTheSameTime runs fn n times concurrently, released together, and returns each error.
func atTheSameTime(n int, fn func(i int) error) []error {
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			<-start
			errs[i] = fn(i)
		})
	}
	close(start)
	wg.Wait()
	return errs
}

// Ping returns nothing but an error, so success is the observable result.
func TestPingAnswersFromARealDatabase(t *testing.T) {
	pool := open(t, testdb.URL(t))

	if err := store.New(pool).Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

// Vercel's Supabase integration appends supa=base-pooler.x to POSTGRES_URL. Postgres
// rejects unknown startup parameters, so the pool must not forward it.
func TestOpenAcceptsTheIntegrationsURLParameters(t *testing.T) {
	pool := open(t, testdb.URL(t)+"&supa=base-pooler.x")

	if err := store.New(pool).Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

// The transaction pooler behind POSTGRES_URL hands each transaction to any server
// connection, so a statement prepared on one is missing on the next (ADR 0006). Queries
// with parameters, which pgx would otherwise prepare, must leave nothing on the server.
func TestParameterisedQueriesLeaveNoPreparedStatementsOnTheServer(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()

	var echoed int
	if err := conn.QueryRow(ctx, "SELECT $1::int", 7).Scan(&echoed); err != nil {
		t.Fatal(err)
	}
	var prepared int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM pg_prepared_statements").Scan(&prepared); err != nil {
		t.Fatal(err)
	}

	if echoed != 7 {
		t.Errorf("echoed %d, want 7", echoed)
	}
	if prepared != 0 {
		t.Errorf("server holds %d prepared statements for this session, want 0", prepared)
	}
}
