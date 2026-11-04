package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Open returns a connection pool for the database at url. It connects lazily, so the
// API starts while the database is unreachable and /health reports it.
func Open(url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	// Vercel's Supabase integration appends supa=base-pooler.x to POSTGRES_URL. pgx would
	// send it as a startup parameter, which Postgres rejects as unknown.
	delete(cfg.ConnConfig.RuntimeParams, "supa")
	// POSTGRES_URL is Supabase's transaction pooler: each transaction may run on a
	// different server connection, so prepared statements can't be relied on (ADR 0006).
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	// Every Vercel instance holds its own pool: keep each one small, and let idle
	// connections go so frozen instances don't hold pooler slots (ADR 0006).
	cfg.MaxConns = 4
	cfg.MaxConnIdleTime = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	return pool, nil
}
