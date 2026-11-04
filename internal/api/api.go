// Package api is the HTTP surface of stock-flow: routes, request and response types,
// and the OpenAPI contract Huma generates from them.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// Store is what the handlers need from the database.
type Store interface {
	Ping(ctx context.Context) error
}

// New builds the HTTP handler and the API it serves. The API's OpenAPI document is
// the published contract (openapi.yaml).
func New(store Store) (http.Handler, huma.API) {
	router := chi.NewMux()
	router.Use(noStore)

	api := humachi.New(router, huma.DefaultConfig("stock-flow API", "0.1.0"))
	registerHealth(api, store)

	return router, api
}

// noStore forbids caching of every response: the web origin reaches this API through a
// Vercel rewrite, and Vercel caches rewritten responses by the upstream Cache-Control
// (ADR 0016).
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
