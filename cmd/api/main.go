// Command api serves the stock-flow HTTP API. On Vercel (Go Framework Preset) every
// instance runs this main and sends traffic to PORT. It never runs migrations: the
// deploy workflow does, once per deploy (ADR 0007).
package main

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/api"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/email"
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
	deps := api.Deps{PasswordHasher: auth.DefaultPasswordHasher, Now: time.Now}
	mailKey, sender := os.Getenv("BREVO_API_KEY"), os.Getenv("EMAIL_SENDER_ADDRESS")
	webBase, codeSecret := os.Getenv("WEB_BASE_URL"), os.Getenv("EMAIL_CODE_SECRET")
	// Legacy-only deployments remain valid; partial email settings must not fail
	// silently and leave a configured-looking but unusable password flow.
	if mailKey != "" || sender != "" || webBase != "" || codeSecret != "" {
		origin, err := url.Parse(webBase)
		if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") || len(codeSecret) < 32 {
			return errors.New("email access requires WEB_BASE_URL as an HTTPS origin and EMAIL_CODE_SECRET of at least 32 bytes")
		}
		mailer, err := email.NewBrevo(email.Config{APIKey: mailKey, SenderEmail: sender}, nil)
		if err != nil {
			return err
		}
		deps.Mailer, deps.CodeSecret, deps.WebBaseURL = mailer, []byte(codeSecret), strings.TrimSuffix(webBase, "/")
	}

	pool, err := store.Open(databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	deps.Store = store.NewStore(pool)
	handler, _ := api.New(deps)
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	slog.Info("api listening", "port", port)
	return server.ListenAndServe()
}
