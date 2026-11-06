package api

import (
	"errors"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
)

// roleOwner is the Role of the one Owner (users-and-access R9).
const roleOwner = "owner"

// ownerOnlyErrors is how every Owner-only operation describes its refusals.
const ownerOnlyErrors = "Owner only: answers 403 to an Admin or Staff. " + sessionErrors

// ownerOnly is the Middleware of an operation only the Owner may use (users-and-access
// R10, R23). Admin is the Owner's equal elsewhere, but not on these operations.
//
// It runs before Huma reads the request, so who is refused never depends on what they
// sent: a visitor hears 401 and an Admin 403, whatever is wrong with the body. The Role
// is read from the User on each request (users-and-access R35).
func ownerOnly(api huma.API, deps Deps) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		var token string
		if cookie, err := huma.ReadCookie(ctx, sessionCookieName); err == nil {
			token = cookie.Value
		}
		session, err := liveSession(ctx.Context(), deps, token)
		if err == nil && session.Role != roleOwner {
			err = huma.Error403Forbidden("Only the Owner can do this.")
		}
		if err == nil {
			next(ctx)
			return
		}
		var problem huma.StatusError
		if !errors.As(err, &problem) {
			problem = internalError(ctx.Context(), "check access", err)
		}
		writeProblem(api, ctx, problem)
	}
}

// writeProblem answers a request from a Middleware, which has no handler to return the
// error from. The body is still Huma's StatusError, so it is the same Problem Details
// as any handler's (ADR 0014).
func writeProblem(api huma.API, ctx huma.Context, problem huma.StatusError) {
	ctx.SetStatus(problem.GetStatus())
	ctx.SetHeader("Content-Type", "application/problem+json")
	if err := api.Marshal(ctx.BodyWriter(), "application/json", problem); err != nil {
		// The status is already sent; all that is left is to say why the body is empty.
		slog.ErrorContext(ctx.Context(), "write problem failed", "err", err)
	}
}
