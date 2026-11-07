package api

import (
	"errors"
	"log/slog"
	"slices"

	"github.com/danielgtaylor/huma/v2"
)

// The Roles an operation can be limited to (users-and-access R8).
const (
	roleOwner = "owner" // the one Owner (R9)
	roleAdmin = "admin"
)

// ownerOnlyErrors is how every Owner-only operation describes its refusals.
const ownerOnlyErrors = "Owner only: answers 403 to an Admin or Staff. " + sessionErrors

// ownerOrAdminErrors is how every operation of the Owner and Admins describes its refusals.
const ownerOrAdminErrors = "Owner and Admins only: answers 403 to Staff. " + sessionErrors

// ownerOnly is the Middleware of an operation only the Owner may use (users-and-access
// R10, R23). Admin is the Owner's equal elsewhere, but not on these operations.
func ownerOnly(api huma.API, deps Deps) func(huma.Context, func(huma.Context)) {
	return requireRole(api, deps, "Only the Owner can do this.", roleOwner)
}

// ownerOrAdmin is the Middleware of an operation the Owner and Admins may use and Staff may
// not (users-and-access R11, R23).
func ownerOrAdmin(api huma.API, deps Deps) func(huma.Context, func(huma.Context)) {
	return requireRole(api, deps, "Only the Owner and Admins can do this.", roleOwner, roleAdmin)
}

// anyRole is the Middleware of an operation every Role may use, which still needs a live
// session (users-and-access R4, R23).
func anyRole(api huma.API, deps Deps) func(huma.Context, func(huma.Context)) {
	return requireRole(api, deps, "")
}

// requireRole refuses the request unless it carries a live session of one of roles, or of
// any Role when none is named. It runs before Huma reads the request, so who is refused
// never depends on what they sent: a visitor hears 401 and a User of another Role 403,
// whatever is wrong with the body. The Role is read from the User on each request
// (users-and-access R35).
func requireRole(api huma.API, deps Deps, forbidden string, roles ...string) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		var token string
		if cookie, err := huma.ReadCookie(ctx, sessionCookieName); err == nil {
			token = cookie.Value
		}
		session, err := liveSession(ctx.Context(), deps, token)
		if err == nil && len(roles) > 0 && !slices.Contains(roles, session.Role) {
			err = huma.Error403Forbidden(forbidden)
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
