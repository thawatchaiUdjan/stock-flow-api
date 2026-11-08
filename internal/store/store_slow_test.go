//go:build slow

package store_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

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

// addProduct seeds a Product with a stock, which the port has no operation to set.
func addProduct(t *testing.T, pool *pgxpool.Pool, product store.Product) store.Product {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`INSERT INTO products (name, sku, unit, price, minimum_stock, stock, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, stock_state`,
		product.Name, product.Sku, product.Unit, product.Price, product.MinimumStock, product.Stock, product.Status,
	).Scan(&product.ID, &product.StockState)
	if err != nil {
		t.Fatal(err)
	}
	return product
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
			Store:      store.NewStore(pool),
			AddUser:    func(t *testing.T, user store.User) store.User { return addUser(t, pool, user) },
			AddProduct: func(t *testing.T, product store.Product) store.Product { return addProduct(t, pool, product) },
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

// products R3.
func TestASKUIsUniqueWhateverItsLetterCase(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	insert := `INSERT INTO products (name, sku, unit, price) VALUES ('Thai tea', $1, 'box', 100)`
	if _, err := pool.Exec(ctx, insert, "tea-01"); err != nil {
		t.Fatal(err)
	}

	_, err := pool.Exec(ctx, insert, "TeA-01")

	assertViolates(t, err, "products_sku_key")
}

// products R3: two Products created with one SKU at the same moment make one Product and
// refuse the other, whatever the letter case.
func TestTwoProductsCreatedAtOnceWithOneSKUMakeOne(t *testing.T) {
	ctx := context.Background()
	s := store.NewStore(open(t, testdb.URL(t)))
	skus := []string{"tea-01", "TEA-01", "Tea-01", "tEA-01", "teA-01", "TEa-01"}
	errs := make(chan error, len(skus))

	var wg sync.WaitGroup
	for _, sku := range skus {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.CreateProduct(ctx, store.InsertProductParams{Name: "Thai tea", Sku: sku, Unit: "box"})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	made, taken := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			made++
		case errors.Is(err, store.ErrSKUTaken):
			taken++
		default:
			t.Errorf("err = %v, want nil or ErrSKUTaken", err)
		}
	}
	if made != 1 || taken != len(skus)-1 {
		t.Errorf("made %d and refused %d, want 1 and %d", made, taken, len(skus)-1)
	}
}

// products R6: stock never goes below 0, whatever the code above does.
func TestTheSchemaRefusesNegativeStock(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))

	_, err := pool.Exec(ctx, `INSERT INTO products (name, sku, unit, price, stock) VALUES ('Tea', 'T', 'box', 1, -1)`)

	assertViolates(t, err, "products_stock_check")
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
	s := store.NewStore(pool)
	link, err := auth.IssueOwnerSetupLink(ctx, s, now, "Malee", "malee@example.com")
	if err != nil {
		t.Fatal(err)
	}
	const attempts = 8
	warm(t, pool)

	used := make([]int64, attempts)
	errs := atTheSameTime(attempts, func(i int) (err error) {
		used[i], err = s.SetPassword(ctx, store.SetPasswordParams{
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
	s := store.NewStore(pool)
	now := time.Now().Truncate(time.Microsecond)
	link, err := auth.IssueOwnerSetupLink(ctx, s, now, "Malee", "malee@example.com")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := s.GetUserByEmail(ctx, "malee@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSession(ctx, store.CreateSessionParams{TokenHash: []byte("laptop"), UserID: owner.ID, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	// The password update fails after the link is deleted, so the link comes back only if
	// the two are one transaction.
	if _, err := pool.Exec(ctx, `ALTER TABLE users ADD CONSTRAINT users_no_boom CHECK (password_hash IS DISTINCT FROM 'boom')`); err != nil {
		t.Fatal(err)
	}

	_, err = s.SetPassword(ctx, store.SetPasswordParams{PasswordHash: "boom", TokenHash: auth.HashToken(link.Token), At: now})

	assertViolates(t, err, "users_no_boom")
	if _, err := s.GetSession(ctx, []byte("laptop")); err != nil {
		t.Errorf("the Owner's session: %v, want it kept", err)
	}
	if _, err := s.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: auth.HashToken(link.Token), At: now}); err != nil {
		t.Errorf("the Setup link: %v, want it kept", err)
	}
}

// context ADR 0003: the other end of that transaction. The Owner's sessions can't be ended,
// so the password isn't set and the link isn't used up: a new password never stands beside
// a session that outlived it.
func TestAPasswordSetThatCannotEndTheOwnersSessionsSetsNoPassword(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	owner := ownerWithSetupLink(t, pool, s)
	if started, err := logIn(ctx, s, owner, "laptop"); err != nil || !started {
		t.Fatalf("started = %v, err = %v", started, err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION refuse_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'refused'; END $$;
		CREATE TRIGGER sessions_refuse_delete BEFORE DELETE ON sessions FOR EACH ROW EXECUTE FUNCTION refuse_delete();
	`); err != nil {
		t.Fatal(err)
	}

	_, err := s.SetPassword(ctx, store.SetPasswordParams{PasswordHash: "new-hash", TokenHash: []byte("link"), At: time.Now()})

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Message != "refused" {
		t.Errorf("err = %v, want the refused delete", err)
	}
	got, err := s.GetUserByEmail(ctx, owner.Email)
	if err != nil {
		t.Fatal(err)
	}
	if got.PasswordHash != hashed {
		t.Errorf("password hash = %+v, want it kept", got.PasswordHash)
	}
	if _, err := s.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: []byte("link"), At: time.Now()}); err != nil {
		t.Errorf("the Setup link: %v, want it kept", err)
	}
}

var hashed = pgtype.Text{String: "hash", Valid: true}

// endsAccess is what the Owner can do to end a User's access, through the real Store.
var endsAccess = map[string]func(ctx context.Context, s *store.Store, id int64) error{
	"Inactive": func(ctx context.Context, s *store.Store, id int64) error {
		_, err := s.UpdateUser(ctx, store.ChangeUserParams{ID: id, Status: "inactive"})
		return err
	},
	"reset": func(ctx context.Context, s *store.Store, id int64) error {
		return s.ResetPassword(ctx, store.IssueSetupLinkParams{UserID: id, TokenHash: []byte("new-link"), ExpiresAt: time.Now().Add(time.Hour)})
	},
}

// users-and-access R16, R31: a login's insert is not committed yet when the Owner acts.
// The Owner's change waits for it, and then finds and ends that session, so it can't
// outlive the change. Without the lock, or with the sessions deleted on the snapshot of a
// single statement, the session would survive.
func TestEndingAUsersAccessEndsASessionBeingStartedAtThatMoment(t *testing.T) {
	for name, end := range endsAccess {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			pool := open(t, testdb.URL(t))
			s := store.NewStore(pool)
			user := addUser(t, pool, store.User{Name: "Somchai", Email: "s@example.com", Role: "staff", Status: "active", PasswordHash: hashed})
			login, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer login.Rollback(ctx)
			started, err := store.New(login).CreateSession(ctx, store.CreateSessionParams{
				TokenHash: []byte("laptop"), UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour), PasswordHash: hashed,
			})
			if err != nil || started != 1 {
				t.Fatalf("started = %d, err = %v", started, err)
			}

			ended := make(chan error, 1)
			go func() { ended <- end(ctx, s, user.ID) }()
			waitUntilBlocked(t, pool)
			if err := login.Commit(ctx); err != nil {
				t.Fatal(err)
			}

			if err := <-ended; err != nil {
				t.Fatal(err)
			}
			if _, err := s.GetSession(ctx, []byte("laptop")); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("session: err = %v, want it ended", err)
			}
		})
	}
}

// users-and-access R16, R31: the other order. The Owner's change is not committed yet when
// the login inserts: the login waits for it, then starts no session.
func TestALoginWaitsForAChangeUnderWayAndThenStartsNoSession(t *testing.T) {
	for name, change := range map[string]func(ctx context.Context, q *store.Queries, id int64) error{
		"Inactive": func(ctx context.Context, q *store.Queries, id int64) error {
			_, err := q.ChangeUser(ctx, store.ChangeUserParams{ID: id, Status: "inactive"})
			return err
		},
		"reset": func(ctx context.Context, q *store.Queries, id int64) error { return q.ClearPassword(ctx, id) },
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			pool := open(t, testdb.URL(t))
			s := store.NewStore(pool)
			user := addUser(t, pool, store.User{Name: "Somchai", Email: "s@example.com", Role: "staff", Status: "active", PasswordHash: hashed})
			owner, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer owner.Rollback(ctx)
			if err := change(ctx, store.New(owner), user.ID); err != nil {
				t.Fatal(err)
			}

			type result struct {
				started int64
				err     error
			}
			login := make(chan result, 1)
			go func() {
				started, err := s.CreateSession(ctx, store.CreateSessionParams{
					TokenHash: []byte("laptop"), UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour), PasswordHash: hashed,
				})
				login <- result{started, err}
			}()
			waitUntilBlocked(t, pool)
			if err := owner.Commit(ctx); err != nil {
				t.Fatal(err)
			}

			if got := <-login; got.err != nil || got.started != 0 {
				t.Errorf("started = %d, err = %v, want 0 and nil", got.started, got.err)
			}
		})
	}
}

// ownerWithSetupLink is an Owner who logs in with hashed and holds the Setup link "link":
// the moment a leaked password is being replaced (users-and-access R34).
func ownerWithSetupLink(t *testing.T, pool *pgxpool.Pool, s *store.Store) store.User {
	t.Helper()
	owner := addUser(t, pool, store.User{Name: "Malee", Email: "malee@example.com", Role: "owner", Status: "active", PasswordHash: hashed})
	err := s.IssueSetupLink(context.Background(), store.IssueSetupLinkParams{UserID: owner.ID, TokenHash: []byte("link"), ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

// logIn is a login whose password check against hashed passed.
func logIn(ctx context.Context, s *store.Store, owner store.User, token string) (bool, error) {
	return s.StartSession(ctx, store.StartSessionParams{
		Session: store.CreateSessionParams{TokenHash: []byte(token), UserID: owner.ID, ExpiresAt: time.Now().Add(time.Hour), PasswordHash: hashed},
		Entry: store.AddLoginLogEntryParams{
			At: time.Now(), Email: owner.Email, UserID: pgtype.Int8{Int64: owner.ID, Valid: true}, Ip: "203.0.113.7", UserAgent: "Firefox",
		},
	})
}

// holdLogin leaves a login's session insert uncommitted, and returns its transaction.
func holdLogin(t *testing.T, pool *pgxpool.Pool, owner store.User, token string) pgx.Tx {
	t.Helper()
	ctx := context.Background()
	login, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { login.Rollback(ctx) })
	started, err := store.New(login).CreateSession(ctx, store.CreateSessionParams{
		TokenHash: []byte(token), UserID: owner.ID, ExpiresAt: time.Now().Add(time.Hour), PasswordHash: hashed,
	})
	if err != nil || started != 1 {
		t.Fatalf("started = %d, err = %v", started, err)
	}
	return login
}

// settingThePassword sets a password through the Setup link "link", and answers once it
// has finished.
func settingThePassword(ctx context.Context, s *store.Store) <-chan error {
	finished := make(chan error, 1)
	go func() {
		used, err := s.SetPassword(ctx, store.SetPasswordParams{PasswordHash: "new-hash", TokenHash: []byte("link"), At: time.Now()})
		if err == nil && used != 1 {
			err = fmt.Errorf("used = %d, want 1", used)
		}
		finished <- err
	}()
	return finished
}

func assertSessionEnded(t *testing.T, s *store.Store, token string) {
	t.Helper()
	if _, err := s.GetSession(context.Background(), []byte(token)); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("session %q: err = %v, want it ended", token, err)
	}
}

// users-and-access R34, context ADR 0003: a login's insert is not committed yet when the
// Owner sets a password. The password set waits for it, and then ends that session too.
func TestSettingTheOwnersPasswordEndsASessionBeingStartedAtThatMoment(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	owner := ownerWithSetupLink(t, pool, s)
	login := holdLogin(t, pool, owner, "laptop")

	finished := settingThePassword(ctx, s)
	waitUntilBlocked(t, pool)
	if err := login.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	assertSessionEnded(t, s, "laptop")
}

// users-and-access R34, context ADR 0003: a login that starts once the password set has
// begun, while it waits for an earlier login. Share locks don't queue behind that wait, so
// this login starts its session and commits before the password is written, and the
// password set has to end both.
func TestSettingTheOwnersPasswordEndsASessionStartedWhileItWaits(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	owner := ownerWithSetupLink(t, pool, s)
	earlier := holdLogin(t, pool, owner, "laptop")

	finished := settingThePassword(ctx, s)
	waitUntilBlocked(t, pool)
	inTime, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	started, err := logIn(inTime, s, owner, "phone")
	if err != nil || !started {
		t.Fatalf("started = %v, err = %v, want a session to end", started, err)
	}
	if err := earlier.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	assertSessionEnded(t, s, "laptop")
	assertSessionEnded(t, s, "phone")
}

// users-and-access R34, R36: a login that starts once the password is written, before the
// password set commits. A logout under way holds the password set at the Owner's sessions,
// which is that moment. The login waits, then finds the password changed: no session
// starts, and the Login log says so.
func TestALoginWaitsForTheOwnersPasswordSetAndThenStartsNoSession(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	owner := ownerWithSetupLink(t, pool, s)
	if started, err := logIn(ctx, s, owner, "laptop"); err != nil || !started {
		t.Fatalf("started = %v, err = %v", started, err)
	}
	logout, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer logout.Rollback(ctx)
	if _, err := store.New(logout).DeleteSession(ctx, []byte("laptop")); err != nil {
		t.Fatal(err)
	}

	finished := settingThePassword(ctx, s)
	waitUntilBlocked(t, pool)
	type result struct {
		started bool
		err     error
	}
	login := make(chan result, 1)
	go func() {
		started, err := logIn(ctx, s, owner, "phone")
		login <- result{started, err}
	}()
	waitUntilQueriesBlocked(t, pool, 2)
	if err := logout.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if got := <-login; got.err != nil || got.started {
		t.Errorf("started = %v, err = %v, want false and nil", got.started, got.err)
	}
	assertSessionEnded(t, s, "phone")
	rows, err := s.ListLoginLog(ctx, store.ListLoginLogParams{RowLimit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Result != store.ResultFailedLogin {
		t.Errorf("newest entries = %+v, want a failed login", rows)
	}
}

// users-and-access R32: an Inactive not committed yet when a reset arrives. The reset waits
// for it, sees the User Inactive, and is refused: a status read without the lock would
// still say Active.
func TestAResetWaitsForAnInactiveUnderWayAndIsThenRefused(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	user := addUser(t, pool, store.User{Name: "Somchai", Email: "s@example.com", Role: "staff", Status: "active", PasswordHash: hashed})
	owner, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Rollback(ctx)
	if _, err := store.New(owner).ChangeUser(ctx, store.ChangeUserParams{ID: user.ID, Status: "inactive"}); err != nil {
		t.Fatal(err)
	}

	reset := make(chan error, 1)
	go func() {
		reset <- s.ResetPassword(ctx, store.IssueSetupLinkParams{UserID: user.ID, TokenHash: []byte("link"), ExpiresAt: time.Now().Add(time.Hour)})
	}()
	waitUntilBlocked(t, pool)
	if err := owner.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err := <-reset; !errors.Is(err, store.ErrUserInactive) {
		t.Errorf("err = %v, want ErrUserInactive", err)
	}
	if links := setupLinks(t, pool, user.ID); links != 0 {
		t.Errorf("an Inactive User has %d Setup links", links)
	}
}

// users-and-access R32: the other order. A reset not committed yet when Inactive arrives:
// Inactive waits for it, then finds the new Setup link and stops it.
func TestAnInactiveWaitsForAResetUnderWayAndStopsItsSetupLink(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	user := addUser(t, pool, store.User{Name: "Somchai", Email: "s@example.com", Role: "staff", Status: "active", PasswordHash: hashed})
	reset, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer reset.Rollback(ctx)
	q := store.New(reset)
	if _, err := q.LockUser(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	link := store.IssueSetupLinkParams{UserID: user.ID, TokenHash: []byte("link"), ExpiresAt: time.Now().Add(time.Hour)}
	if err := q.IssueSetupLink(ctx, link); err != nil {
		t.Fatal(err)
	}

	inactive := make(chan error, 1)
	go func() {
		_, err := s.UpdateUser(ctx, store.ChangeUserParams{ID: user.ID, Status: "inactive"})
		inactive <- err
	}()
	waitUntilBlocked(t, pool)
	if err := reset.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err := <-inactive; err != nil {
		t.Fatal(err)
	}
	if links := setupLinks(t, pool, user.ID); links != 0 {
		t.Errorf("an Inactive User has %d Setup links", links)
	}
}

// users-and-access R16, R32: a person sets a password at the moment the Owner acts.
// SetPassword takes the link and then the User. The Owner's change has to take them in the
// same order, or each holds what the other wants and Postgres aborts one of them.
func TestSettingAPasswordAndEndingAccessAtOnceDoNotDeadlock(t *testing.T) {
	for name, end := range endsAccess {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			pool := open(t, testdb.URL(t))
			s := store.NewStore(pool)
			user := addUser(t, pool, store.User{Name: "Somchai", Email: "s@example.com", Role: "staff", Status: "active"})
			err := s.IssueSetupLink(ctx, store.IssueSetupLinkParams{UserID: user.ID, TokenHash: []byte("link"), ExpiresAt: time.Now().Add(time.Hour)})
			if err != nil {
				t.Fatal(err)
			}
			// SetPassword's first half: the link is taken, the User is not yet.
			person, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer person.Rollback(ctx)
			q := store.New(person)
			if err := q.DeleteSetupLink(ctx, user.ID); err != nil {
				t.Fatal(err)
			}

			ended := make(chan error, 1)
			go func() { ended <- end(ctx, s, user.ID) }()
			waitUntilBlocked(t, pool)
			// Its second half: this is where the two would deadlock.
			if err := q.ClearPassword(ctx, user.ID); err != nil {
				t.Fatalf("the person's half: %v", err)
			}
			if err := person.Commit(ctx); err != nil {
				t.Fatal(err)
			}

			if err := <-ended; err != nil {
				t.Errorf("the Owner's half: %v", err)
			}
		})
	}
}

func setupLinks(t *testing.T, pool *pgxpool.Pool, userID int64) (links int) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `SELECT count(*) FROM setup_links WHERE user_id = $1`, userID).Scan(&links)
	if err != nil {
		t.Fatal(err)
	}
	return links
}

// A User change and its deletes commit together: one that fails partway ends nothing.
func TestAResetThatFailsPartwayChangesNothing(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	s := store.NewStore(pool)
	user := addUser(t, pool, store.User{Name: "Somchai", Email: "s@example.com", Role: "staff", Status: "active", PasswordHash: hashed})
	other := addUser(t, pool, store.User{Name: "Other", Email: "o@example.com", Role: "staff", Status: "active"})
	expires := time.Now().Add(time.Hour)
	if _, err := s.CreateSession(ctx, store.CreateSessionParams{TokenHash: []byte("laptop"), UserID: user.ID, ExpiresAt: expires, PasswordHash: hashed}); err != nil {
		t.Fatal(err)
	}
	if err := s.IssueSetupLink(ctx, store.IssueSetupLinkParams{UserID: other.ID, TokenHash: []byte("taken"), ExpiresAt: expires}); err != nil {
		t.Fatal(err)
	}

	err := s.ResetPassword(ctx, store.IssueSetupLinkParams{UserID: user.ID, TokenHash: []byte("taken"), ExpiresAt: expires})

	assertViolates(t, err, "setup_links_token_hash_key")
	got, err := s.GetUserByEmail(ctx, user.Email)
	if err != nil {
		t.Fatal(err)
	}
	if got.PasswordHash != hashed {
		t.Errorf("password hash = %+v, want it kept", got.PasswordHash)
	}
	if _, err := s.GetSession(ctx, []byte("laptop")); err != nil {
		t.Errorf("session: %v, want it kept", err)
	}
}

// waitUntilBlocked returns once a query is waiting for a lock, so what a test does next
// happens while that query waits.
func waitUntilBlocked(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	waitUntilQueriesBlocked(t, pool, 1)
}

// waitUntilQueriesBlocked is waitUntilBlocked for n queries at once.
func waitUntilQueriesBlocked(t *testing.T, pool *pgxpool.Pool, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		err := pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_locks WHERE NOT granted`).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("fewer than %d queries blocked on a lock", n)
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

// users-and-access R39: the schema refuses to edit or remove an entry, whoever asks.
func TestTheSchemaRefusesToChangeOrRemoveALoginLogEntry(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	if err := store.New(pool).AddLoginLogEntry(ctx, store.AddLoginLogEntryParams{
		At: time.Now(), Email: "nobody@example.com", Result: store.ResultFailedLogin, Ip: "203.0.113.7", UserAgent: "Firefox",
	}); err != nil {
		t.Fatal(err)
	}

	for name, statement := range map[string]string{
		"update":   `UPDATE login_log SET email = 'someone@example.com'`,
		"delete":   `DELETE FROM login_log`,
		"truncate": `TRUNCATE login_log`,
	} {
		_, err := pool.Exec(ctx, statement)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23001" {
			t.Errorf("%s: err = %v, want a restrict_violation", name, err)
		}
	}
	var email string
	if err := pool.QueryRow(ctx, `SELECT email FROM login_log`).Scan(&email); err != nil || email != "nobody@example.com" {
		t.Errorf("entry = %q, %v, want it untouched", email, err)
	}
}

// products R2, R4: a name or unit of only whitespace in any script is refused by the
// schema itself, and the Sales queries have an index on completed_at. The migration that
// adds both applies over data already stored, and its down puts the schema back.
func TestTheBlankProductAndSalesIndexMigrationAppliesAndReverses(t *testing.T) {
	ctx := context.Background()
	url := testdb.URL(t)
	pool := open(t, url)
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS("../../db/migrations"))
	if err != nil {
		t.Fatal(err)
	}
	const insert = `INSERT INTO products (name, sku, unit, price) VALUES ($1, $2, $3, 0)`
	const hasIndex = `SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'orders_completed_at_idx')`

	// Back to the schema before the migration, holding a valid Product.
	if _, err := provider.DownTo(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, insert, "Thai tea", "TEA-01", "pcs"); err != nil {
		t.Fatal(err)
	}
	var indexed bool
	if err := pool.QueryRow(ctx, hasIndex).Scan(&indexed); err != nil || indexed {
		t.Fatalf("before the migration: indexed = %v, err = %v, want no index", indexed, err)
	}

	// Up: the stored Product stays, blank names and units in any script are refused.
	if _, err := provider.UpByOne(ctx); err != nil {
		t.Fatal(err)
	}
	for _, blank := range []string{"\u00a0", "\u3000", "\u0085", "\v", " \u2003\u2028 ", ""} {
		_, err := pool.Exec(ctx, insert, blank, "BLANK-1", "pcs")
		assertViolates(t, err, "products_name_check")
		_, err = pool.Exec(ctx, insert, "Tea", "BLANK-2", blank)
		assertViolates(t, err, "products_unit_check")
	}
	if _, err := pool.Exec(ctx, insert, "ชาไทย", "TEA-02", "ชิ้น"); err != nil {
		t.Errorf("a name and unit in Thai: %v, want them stored", err)
	}
	if _, err := pool.Exec(ctx, insert, "\u3000Tea\u00a0", "TEA-03", "6 pack"); err != nil {
		t.Errorf("a name with a space around it: %v, want it stored", err)
	}
	if err := pool.QueryRow(ctx, hasIndex).Scan(&indexed); err != nil || !indexed {
		t.Errorf("after the migration: indexed = %v, err = %v, want the index", indexed, err)
	}

	// Down: the index goes, and so does the wider refusal.
	if _, err := provider.Down(ctx); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, hasIndex).Scan(&indexed); err != nil || indexed {
		t.Errorf("after down: indexed = %v, err = %v, want no index", indexed, err)
	}
	if _, err := pool.Exec(ctx, insert, "\u3000", "OLD-2", "pcs"); err != nil {
		t.Errorf("after down, an ideographic space name: %v, want it stored again", err)
	}
	_, err = pool.Exec(ctx, insert, " ", "OLD-3", "pcs")
	assertViolates(t, err, "products_name_check")
}
