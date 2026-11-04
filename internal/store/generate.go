// Package store is the database: sqlc generates the queries from db/queries, against
// the schema the goose migrations in db/migrations build.
package store

//go:generate go tool sqlc generate -f ../../sqlc.yaml
