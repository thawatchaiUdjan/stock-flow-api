//go:build slow

package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/testdb"
)

func TestMain(m *testing.M) { os.Exit(testdb.Main(m)) }

func TestPingAnswersFromARealDatabase(t *testing.T) {
	pool, err := store.Open(testdb.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	if err := store.New(pool).Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

// The transaction pooler behind POSTGRES_URL hands each transaction to any server
// connection, so a statement prepared on one is missing on the next (ADR 0006). The pool
// must therefore leave no prepared statement behind on the server.
func TestQueriesLeaveNoPreparedStatementsOnTheServer(t *testing.T) {
	ctx := context.Background()
	pool, err := store.Open(testdb.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()

	if err := store.New(conn).Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	var prepared int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM pg_prepared_statements").Scan(&prepared); err != nil {
		t.Fatal(err)
	}

	if prepared != 0 {
		t.Errorf("server holds %d prepared statements for this session, want 0", prepared)
	}
}
