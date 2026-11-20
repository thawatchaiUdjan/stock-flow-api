package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

type identifyOutput struct {
	Body struct {
		Next         string `json:"next" enum:"password,first-time-code,no-account,installation-link"`
		CodeDelivery string `json:"codeDelivery,omitempty" enum:"sent,failed,suppressed" doc:"Only with first-time-code. sent means provider acceptance, not inbox delivery; suppressed means no new code went out because of the send limits; failed means the provider refused. A previous unexpired code keeps working."`
	}
}

// registerLoginIdentify serves Login's email step (business ADR 0012).
func registerLoginIdentify(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "identify-login-email", Method: http.MethodPost, Path: "/v2/login/identify",
		Summary: "Say what an email may do next on Login", Tags: []string{"Email access"},
		Description: "Login's email step. Answers `password` for an Active User with a password; `no-account` alike for an unknown email and an Inactive User; `installation-link` for the Owner before installation setup, sending nothing; and `first-time-code` for an Active Admin or Staff User without a password, after sending a First-time code within the shared send limits (`codeDelivery` says whether one went out). " +
			"Answers 429 `login-check-limited` with Retry-After after 100 checks from one IP address in a rolling hour, and 503 `email-auth-unavailable` only where a code would be sent and email access is not configured. Not recorded in the Login log.",
		Errors: []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable},
	}, func(ctx context.Context, input *emailRequestInput) (*identifyOutput, error) {
		if err := reserveLoginCheck(ctx, deps); err != nil {
			return nil, err
		}
		out := &identifyOutput{}
		user, err := deps.Store.GetUserByEmail(ctx, input.Body.Email)
		switch {
		case errors.Is(err, store.ErrNotFound) || err == nil && user.Status != statusActive:
			// users-and-access R50: Inactive status is never told apart from an unknown email.
			out.Body.Next = "no-account"
		case err != nil:
			return nil, internalError(ctx, "identify login email", err)
		case user.PasswordHash.Valid:
			out.Body.Next = "password"
		case user.Role == roleOwner:
			// users-and-access R34, R52: initial Owner setup sends nothing.
			out.Body.Next = "installation-link"
		default:
			if err := emailAuthReady(deps); err != nil {
				return nil, err
			}
			// users-and-access R51: the code step is still the answer when no new code went out.
			delivery, err := sendPasswordInstructions(ctx, deps, user.Email, false)
			if err != nil {
				return nil, internalError(ctx, "send first-time code from login", err)
			}
			out.Body.Next, out.Body.CodeDelivery = "first-time-code", delivery
		}
		return out, nil
	})
}

// reserveLoginCheck applies users-and-access R53 before any lookup, so a limited answer
// is the same whatever the email.
func reserveLoginCheck(ctx context.Context, deps Deps) error {
	info, _ := ctx.Value(requestInfoKey{}).(requestInfo)
	now := deps.Now()
	allowed, retryAt, err := deps.Store.ReserveLoginCheck(ctx, store.LoginCheckParams{IPHash: auth.HashToken(info.ip), At: now})
	if err != nil {
		return internalError(ctx, "reserve login check", err)
	}
	if allowed {
		return nil
	}
	return retryLater(codeLoginCheckLimited, "Too many email checks from this network. Try again later.", retryAt, now)
}

// retryLater is a 429 domain error whose Retry-After says when to try again.
func retryLater(code, detail string, retryAt, now time.Time) error {
	seconds := max(1, int(math.Ceil(retryAt.Sub(now).Seconds())))
	return huma.ErrorWithHeaders(
		domainError(http.StatusTooManyRequests, code, detail),
		http.Header{"Retry-After": {strconv.Itoa(seconds)}},
	)
}
