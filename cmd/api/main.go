// Command api serves the stock-flow HTTP API. On Vercel (Go Framework Preset) every
// instance runs this main and sends traffic to PORT. It never runs migrations: the
// deploy workflow does, once per deploy (ADR 0007).
package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/api"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	// Local runs read .env; on Vercel the file doesn't exist and the environment is set.
	// Variables already in the environment win.
	_ = godotenv.Load()

	databaseURL := os.Getenv("POSTGRES_URL")
	if databaseURL == "" {
		return errors.New("POSTGRES_URL is not set (copy .env.example to .env for local runs)")
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	pool, err := store.Open(databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	handler, _ := api.New(api.Deps{Store: store.New(pool), PasswordHasher: auth.DefaultPasswordHasher, Now: time.Now})
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	slog.Info("api listening", "port", port)
	return server.ListenAndServe()
}
