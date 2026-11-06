// Command install is run by a developer, outside the web app. It creates the Owner when
// there is none and prints the Owner's Setup link token; run again, it issues a new token
// and the earlier one stops working. It never takes or prints a password
// (users-and-access R34).
//
//	go run ./cmd/install -name "<name>" -email "<email>"   first time
//	go run ./cmd/install                                   the Owner exists
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/joho/godotenv"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "install:", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()
	databaseURL := os.Getenv("POSTGRES_URL")
	if databaseURL == "" {
		return errors.New("POSTGRES_URL is not set (copy .env.example to .env for local runs)")
	}
	pool, err := store.Open(databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	ctx := context.Background()
	return install(os.Args[1:], os.Stdout, func(name, email string) (link auth.SetupLink, err error) {
		err = store.InTx(ctx, pool, func(q *store.Queries) (err error) {
			link, err = auth.IssueOwnerSetupLink(ctx, q, time.Now(), name, email)
			return err
		})
		return link, err
	})
}

type issueFunc func(name, email string) (auth.SetupLink, error)

// install prints only after issue returns, so a token is never shown that didn't commit.
func install(args []string, out io.Writer, issue issueFunc) error {
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	flags.SetOutput(out)
	name := flags.String("name", "", "the Owner's name, to create the Owner")
	email := flags.String("email", "", "the Owner's email, to create the Owner")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: only -name and -email are taken")
	}

	link, err := issue(*name, *email)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Setup link token for the Owner, %s. It works once, until %s:\n%s\n",
		link.Email, link.ExpiresAt.UTC().Format(time.RFC3339), link.Token)
	return nil
}
