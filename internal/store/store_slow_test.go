//go:build slow

package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
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
