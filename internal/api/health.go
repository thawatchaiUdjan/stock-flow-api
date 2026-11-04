package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type healthOutput struct {
	Body struct {
		Status string `json:"status" example:"ok" doc:"Always \"ok\" when the API and its database answer."`
	}
}

// registerHealth adds GET /health. It queries the database on purpose: the daily
// keepalive ping then counts as activity, and a Supabase Free project pauses after a
// week without any (ADR 0010).
func registerHealth(api huma.API, store Store) {
	huma.Register(api, huma.Operation{
		OperationID: "get-health",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Check that the API and its database answer",
		Tags:        []string{"Health"},
		Errors:      []int{http.StatusServiceUnavailable},
	}, func(ctx context.Context, _ *struct{}) (*healthOutput, error) {
		if err := store.Ping(ctx); err != nil {
			slog.ErrorContext(ctx, "health: database ping failed", "err", err)
			return nil, huma.Error503ServiceUnavailable("database unreachable")
		}
		out := &healthOutput{}
		out.Body.Status = "ok"
		return out, nil
	})
}
