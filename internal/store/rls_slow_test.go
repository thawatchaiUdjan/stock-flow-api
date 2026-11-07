//go:build slow

package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/api"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/testdb"
)

// assertRoleOnlyOwnsTheTables fails unless the pool's role passes Row Level Security by
// owning the tables and nothing else. A superuser or a BYPASSRLS role would pass whatever
// the migrations did, and a test run as one would prove nothing about POSTGRES_URL's role.
func assertRoleOnlyOwnsTheTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var role string
	var super, bypass bool
	err := pool.QueryRow(ctx,
		`SELECT rolname, rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`,
	).Scan(&role, &super, &bypass)
	if err != nil {
		t.Fatal(err)
	}
	if super || bypass {
		t.Fatalf("role %s: superuser = %v, BYPASSRLS = %v, want neither", role, super, bypass)
	}
	for _, table := range []string{"users", "sessions", "setup_links", "login_log", "products", "stock_movements"} {
		var owner string
		err := pool.QueryRow(ctx, `SELECT tableowner FROM pg_tables WHERE schemaname = 'public' AND tablename = $1`, table).Scan(&owner)
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if owner != role {
			t.Errorf("%s is owned by %s, want %s", table, owner, role)
		}
	}
}

// ADR 0021: every table of ours has Row Level Security on and no policy, so nothing
// reaches it through Supabase's Data API. Not forced, or the owner would be shut out too.
// goose's own version table holds nothing of the Shop's.
func TestEveryTableHasRowLevelSecurityAndNoPolicy(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))

	rows, err := pool.Query(ctx, `
		SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relname <> 'goose_db_version'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		var enabled, forced bool
		if err := rows.Scan(&name, &enabled, &forced); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
		if !enabled || forced {
			t.Errorf("%s: enabled = %v, forced = %v, want true and false", name, enabled, forced)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"users", "sessions", "setup_links", "login_log", "products", "stock_movements"} {
		if !slices.Contains(tables, want) {
			t.Errorf("no table %s in %v", want, tables)
		}
	}

	var policies int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_policies WHERE schemaname = 'public'`).Scan(&policies); err != nil {
		t.Fatal(err)
	}
	if policies != 0 {
		t.Errorf("%d policies, want none: authorization stays in Go (ADR 0015)", policies)
	}
}

// ADR 0021: the anon key reaches the database as the role anon, which Supabase grants
// everything on public tables. The same goes for any role that neither owns the tables
// nor has BYPASSRLS, so this is also what POSTGRES_URL would get with a wrong role:
// reads come back empty, and a write is refused out loud.
func TestARoleThatDoesNotOwnTheTablesReachesNothing(t *testing.T) {
	ctx := context.Background()
	url := testdb.URL(t)
	pool := open(t, url)
	q := store.New(pool)
	link, err := auth.IssueOwnerSetupLink(ctx, q, time.Now(), "Malee", "malee@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateSession(ctx, store.CreateSessionParams{
		TokenHash: []byte("session"), UserID: mustOwner(t, q).ID, ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	admin := open(t, testdb.AdminURL(url))
	for _, statement := range []string{
		`CREATE ROLE anon NOLOGIN`,
		`GRANT ALL ON ALL TABLES IN SCHEMA public TO anon`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, statement := range []string{`REVOKE ALL ON ALL TABLES IN SCHEMA public FROM anon`, `DROP ROLE anon`} {
			if _, err := admin.Exec(context.Background(), statement); err != nil {
				t.Error(err)
			}
		}
	})
	asAnon := func(fn func(tx pgx.Tx) error) error {
		return pgx.BeginFunc(ctx, admin, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SET LOCAL ROLE anon`); err != nil {
				return err
			}
			return fn(tx)
		})
	}

	for _, table := range []string{"users", "sessions", "setup_links", "login_log", "products", "stock_movements"} {
		var rows int
		err := asAnon(func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&rows)
		})
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if rows != 0 {
			t.Errorf("anon read %d rows of %s, want 0", rows, table)
		}
	}

	err = asAnon(func(tx pgx.Tx) error {
		return store.New(tx).CreateOwner(ctx, store.CreateOwnerParams{Name: "Other", Email: "other@example.com"})
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Errorf("anon writing a User: err = %v, want a row-level security refusal", err)
	}

	var changed int64
	err = asAnon(func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE users SET password_hash = 'taken'`)
		changed = tag.RowsAffected()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if changed != 0 {
		t.Errorf("anon changed %d Users, want 0", changed)
	}

	// Nothing above touched what the owner sees.
	if _, err := q.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: auth.HashToken(link.Token), At: time.Now()}); err != nil {
		t.Errorf("the owner lost the Setup link: %v", err)
	}
}

func mustOwner(t *testing.T, q *store.Queries) store.User {
	t.Helper()
	owner, err := q.GetOwner(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

// ADR 0021: POSTGRES_URL's role owns the tables, and that alone must keep the whole path
// working with Row Level Security on. Run as a role with no other way past it.
func TestTheAPIWorksAsARoleThatOnlyOwnsTheTables(t *testing.T) {
	ctx := context.Background()
	pool := open(t, testdb.URL(t))
	assertRoleOnlyOwnsTheTables(t, pool)
	handler, _ := api.New(api.Deps{
		Store:          store.NewStore(pool),
		PasswordHasher: auth.PasswordHasher{Memory: 8, Time: 1},
		Now:            time.Now,
	})
	call := func(method, path string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		var payload bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&payload).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, &payload)
		req.Header.Set("Content-Type", "application/json")
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	want := func(step string, rec *httptest.ResponseRecorder, status int) {
		t.Helper()
		if rec.Code != status {
			t.Fatalf("%s: status = %d, want %d; body %s", step, rec.Code, status, rec.Body)
		}
	}

	var link auth.SetupLink
	err := store.InTx(ctx, pool, func(q *store.Queries) (err error) {
		link, err = auth.IssueOwnerSetupLink(ctx, q, time.Now(), "Malee", "malee@example.com")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	want("read setup link", call(http.MethodPost, "/setup-link/read", map[string]string{"token": link.Token}), http.StatusOK)
	want("set password", call(http.MethodPost, "/setup-link/password",
		map[string]string{"token": link.Token, "password": "correct horse"}), http.StatusNoContent)

	login := call(http.MethodPost, "/login", map[string]string{"email": "MALEE@example.com", "password": "correct horse"})
	want("log in", login, http.StatusOK)
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("log in set %d cookies, want 1", len(cookies))
	}
	session := &http.Cookie{Name: cookies[0].Name, Value: cookies[0].Value}

	me := call(http.MethodGet, "/me", nil, session)
	want("get current user", me, http.StatusOK)
	var user struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	if user.Email != "malee@example.com" || user.Role != "owner" {
		t.Errorf("current user = %+v", user)
	}

	want("log out", call(http.MethodPost, "/logout", nil, session), http.StatusNoContent)
	want("after log out", call(http.MethodGet, "/me", nil, session), http.StatusUnauthorized)
}
