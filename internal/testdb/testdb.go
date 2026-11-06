// Package testdb gives slow tests a real Postgres: one embedded server per test
// package, and a fresh, migrated database cloned from a template for each test
// (ADR 0011).
package testdb

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	postgres "github.com/fergusstrange/embedded-postgres/v2"
	_ "github.com/jackc/pgx/v5/stdlib" // pgtestdb connects through database/sql
	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/goosemigrator"
)

var (
	server pgtestdb.Config
	// An fs.FS rooted at the migrations: the migrator's own path handling expects a
	// slash-separated relative path, which an absolute Windows path is not.
	migrator = goosemigrator.New(".", goosemigrator.WithFS(os.DirFS(migrationsDir())))
)

// Main starts the package's Postgres, runs its tests and stops it. Call it from
// TestMain: func TestMain(m *testing.M) { os.Exit(testdb.Main(m)) }.
func Main(m *testing.M) int {
	pg := postgres.NewDatabase(postgres.DefaultConfig().
		Version(postgres.V17). // the major version Supabase runs
		Port(0).
		// Durability buys nothing in a throwaway database and costs most of its time.
		StartParameters(map[string]string{
			"fsync":              "off",
			"synchronous_commit": "off",
			"full_page_writes":   "off",
		}))
	if err := pg.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "testdb: start embedded postgres:", err)
		return 1
	}
	defer func() {
		if err := pg.Stop(); err != nil {
			fmt.Fprintln(os.Stderr, "testdb: stop embedded postgres:", err)
		}
	}()

	server = pgtestdb.Config{
		DriverName: "pgx",
		Host:       "localhost",
		Port:       strconv.FormatUint(uint64(pg.GetPort()), 10),
		User:       "postgres",
		Password:   "postgres",
		Database:   "postgres",
		Options:    "sslmode=disable",
	}
	return m.Run()
}

// URL returns the connection URL of a fresh database, migrated to the latest version,
// that exists only for this test.
func URL(t testing.TB) string {
	t.Helper()
	return pgtestdb.Custom(t, server, migrator).URL()
}

// AdminURL returns databaseURL connecting as the server's superuser, for what the role of
// URL may not do, such as creating another role.
func AdminURL(databaseURL string) string {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		panic(fmt.Sprintf("testdb: parse %q: %v", databaseURL, err))
	}
	parsed.User = url.UserPassword(server.User, server.Password)
	return parsed.String()
}

func migrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "db", "migrations")
}
