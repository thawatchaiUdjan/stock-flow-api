package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/email"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

const emailRequestMessage = "If this email is eligible, we've sent instructions."

type emailRequestInput struct {
	Body struct {
		Email string `json:"email" format:"email"`
	}
}

type emailRequestOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

type createEmailUserOutput struct {
	Body struct {
		User          user   `json:"user"`
		EmailDelivery string `json:"emailDelivery" enum:"sent,failed,suppressed" doc:"sent means provider acceptance, not inbox delivery. Creation succeeds even when sending fails or is limited. Retry through password setup; do not create a duplicate User."`
	}
}

type verifyFirstTimeCodeInput struct {
	Body struct {
		Email string `json:"email" format:"email"`
		Code  string `json:"code" pattern:"^[0-9]{6}$" minLength:"6" maxLength:"6"`
	}
}

type verifyFirstTimeCodeOutput struct {
	Body struct {
		Token     string    `json:"token" doc:"Single-use password-setup grant. Keep out of paths, query strings and logs."`
		ExpiresAt time.Time `json:"expiresAt"`
	}
}

type emailPasswordInput struct {
	Body struct {
		Token string `json:"token" doc:"The single-use credential received from verification or an email link."`
		// users-and-access R19: retain the existing password policy.
		Password string `json:"password" minLength:"6"`
	}
}

func emailAuthReady(deps Deps) error {
	if deps.Mailer == nil || len(deps.CodeSecret) < 32 || deps.WebBaseURL == "" {
		return domainError(http.StatusServiceUnavailable, codeEmailAuthUnavailable, "Email password setup and recovery are not configured.")
	}
	return nil
}

func registerEmailAuth(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "create-email-user", Method: http.MethodPost, Path: "/v2/users",
		Summary: "Create a User and email their First-time code", Tags: []string{"Email access"},
		Description: "Answers 409 `email-taken` when the address is already registered, and 503 `email-auth-unavailable` when email access is not configured. Creation remains successful if sending fails. " + ownerOnlyErrors,
		Security:    sessionSecurity, Middlewares: huma.Middlewares{ownerOnly(api, deps)},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusInternalServerError, http.StatusServiceUnavailable},
	}, func(ctx context.Context, input *createUserInput) (*createEmailUserOutput, error) {
		if err := emailAuthReady(deps); err != nil {
			return nil, err
		}
		created, err := deps.Store.CreateEmailUser(ctx, store.CreateEmailUserParams{
			Name: input.Body.Name, Email: input.Body.Email, Role: input.Body.Role,
		})
		if errors.Is(err, store.ErrNotFound) {
			return nil, domainError(http.StatusConflict, codeEmailTaken, "A User with this email already exists.")
		}
		if err != nil {
			return nil, internalError(ctx, "create email User", err)
		}
		// users-and-access R46: delivery failure cannot undo creation or invite a duplicate retry.
		delivery, err := sendPasswordInstructions(ctx, deps, created.Email, false)
		if err != nil {
			delivery = "failed"
			slog.WarnContext(ctx, "initial password email could not be prepared")
		}
		out := &createEmailUserOutput{}
		out.Body.User = user{ID: created.ID, Name: created.Name, Email: created.Email, Role: created.Role, Status: created.Status}
		out.Body.EmailDelivery = delivery
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "request-first-time-code", Method: http.MethodPost, Path: "/v2/password-setup/request",
		Summary: "Request a First-time code", Tags: []string{"Email access"},
		Description:   "Always answers the same 202 message for eligibility, throttling and delivery outcomes. Answers 503 `email-auth-unavailable` when email access is not configured.",
		DefaultStatus: http.StatusAccepted, Errors: []int{http.StatusInternalServerError, http.StatusServiceUnavailable},
	}, func(ctx context.Context, input *emailRequestInput) (*emailRequestOutput, error) {
		if err := emailAuthReady(deps); err != nil {
			return nil, err
		}
		if _, err := sendPasswordInstructions(ctx, deps, input.Body.Email, false); err != nil {
			return nil, internalError(ctx, "request first-time code", err)
		}
		out := &emailRequestOutput{}
		out.Body.Message = emailRequestMessage
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "verify-first-time-code", Method: http.MethodPost, Path: "/v2/password-setup/verify",
		Summary: "Exchange a First-time code for a password-setup grant", Tags: []string{"Email access"},
		Description: "Answers 410 `first-time-code-invalid` for wrong, expired, replaced, consumed, blocked or ineligible codes; answers 503 `email-auth-unavailable` when not configured. No session starts.",
		Errors:      []int{http.StatusGone, http.StatusInternalServerError, http.StatusServiceUnavailable},
	}, func(ctx context.Context, input *verifyFirstTimeCodeInput) (*verifyFirstTimeCodeOutput, error) {
		if err := emailAuthReady(deps); err != nil {
			return nil, err
		}
		now := deps.Now()
		token := auth.NewToken()
		expires := now.Add(10 * time.Minute)
		verified, err := deps.Store.VerifyFirstTimeCode(ctx, store.VerifyFirstTimeCodeParams{
			Email: input.Body.Email, CodeHash: auth.HashFirstTimeCode(deps.CodeSecret, input.Body.Email, input.Body.Code),
			GrantHash: auth.HashToken(token), GrantExpiresAt: expires, At: now,
		})
		if err != nil {
			return nil, internalError(ctx, "verify first-time code", err)
		}
		if !verified {
			return nil, domainError(http.StatusGone, codeFirstTimeCodeInvalid, "This code cannot be used. Request fresh instructions or try again later.")
		}
		out := &verifyFirstTimeCodeOutput{}
		out.Body.Token, out.Body.ExpiresAt = token, expires
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "set-email-password", Method: http.MethodPost, Path: "/v2/password-setup/password",
		Summary: "Set a first password using a verified grant", Tags: []string{"Email access"},
		Description:   "Answers 410 `password-setup-invalid` for expired, consumed, replaced or ineligible grants, and 503 `email-auth-unavailable` when not configured. Starts no session.",
		DefaultStatus: http.StatusNoContent, Errors: []int{http.StatusGone, http.StatusInternalServerError, http.StatusServiceUnavailable},
	}, func(ctx context.Context, input *emailPasswordInput) (*struct{}, error) {
		_, err := saveEmailPassword(ctx, deps, input, store.SetupGrant)
		return nil, err
	})

	huma.Register(api, huma.Operation{
		OperationID: "request-password-reset", Method: http.MethodPost, Path: "/v2/password-reset/request",
		Summary: "Request self-service password recovery", Tags: []string{"Email access"},
		Description:   "Answers the same 202 message for unknown, inactive, limited and delivery-failure outcomes. Users without passwords receive a First-time code, except the initial Owner. Answers 503 `email-auth-unavailable` when not configured.",
		DefaultStatus: http.StatusAccepted, Errors: []int{http.StatusInternalServerError, http.StatusServiceUnavailable},
	}, func(ctx context.Context, input *emailRequestInput) (*emailRequestOutput, error) {
		if err := emailAuthReady(deps); err != nil {
			return nil, err
		}
		if _, err := sendPasswordInstructions(ctx, deps, input.Body.Email, true); err != nil {
			return nil, internalError(ctx, "request password reset", err)
		}
		out := &emailRequestOutput{}
		out.Body.Message = emailRequestMessage
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "reset-email-password", Method: http.MethodPost, Path: "/v2/password-reset/password",
		Summary: "Set a new password using an emailed Reset link", Tags: []string{"Email access"},
		Description:   "Answers 410 `password-reset-invalid` for expired, used, replaced or ineligible links, and 503 `email-auth-unavailable` when not configured. Ends all sessions and starts none. Notification failure does not undo the change.",
		DefaultStatus: http.StatusNoContent, Errors: []int{http.StatusGone, http.StatusInternalServerError, http.StatusServiceUnavailable},
	}, func(ctx context.Context, input *emailPasswordInput) (*struct{}, error) {
		updated, err := saveEmailPassword(ctx, deps, input, store.ResetLink)
		if err != nil {
			return nil, err
		}
		if err := deps.Mailer.Send(ctx, email.Message{Kind: email.ResetComplete, To: updated.Email, Name: updated.Name, Link: deps.WebBaseURL + "/login"}); err != nil {
			slog.WarnContext(ctx, "password reset notification could not be sent")
		}
		return nil, nil
	})
}

func saveEmailPassword(ctx context.Context, deps Deps, input *emailPasswordInput, kind string) (store.User, error) {
	if err := emailAuthReady(deps); err != nil {
		return store.User{}, err
	}
	code, detail := codePasswordSetupInvalid, "This password-setup grant no longer works. Request a new code."
	if kind == store.ResetLink {
		code, detail = codePasswordResetInvalid, "This Reset link no longer works. Request a new one through Forgot password."
	}
	refusal := func(doing string, err error) error {
		if errors.Is(err, store.ErrNotFound) {
			return domainError(http.StatusGone, code, detail)
		}
		return internalError(ctx, doing, err)
	}
	arg := store.EmailPasswordParams{Kind: kind, TokenHash: auth.HashToken(input.Body.Token), At: deps.Now()}
	if _, err := deps.Store.ReadEmailPasswordUser(ctx, arg); err != nil {
		return store.User{}, refusal("read password credential", err)
	}
	arg.PasswordHash = deps.PasswordHasher.Hash(input.Body.Password)
	arg.At = deps.Now()
	updated, err := deps.Store.SetEmailPassword(ctx, arg)
	if err != nil {
		return store.User{}, refusal("save email password", err)
	}
	return updated, nil
}

func sendPasswordInstructions(ctx context.Context, deps Deps, address string, recovery bool) (string, error) {
	info, _ := ctx.Value(requestInfoKey{}).(requestInfo)
	allowed, err := deps.Store.ReserveEmailRequest(ctx, store.EmailRequestParams{
		EmailHash: auth.HashToken(strings.ToLower(address)), IPHash: auth.HashToken(info.ip), At: deps.Now(),
	})
	if err != nil {
		return "failed", err
	}
	if !allowed {
		return "suppressed", nil
	}
	code := auth.NewFirstTimeCode()
	expires := deps.Now().Add(10 * time.Minute)
	kind := store.FirstTimeCode
	digest := auth.HashFirstTimeCode(deps.CodeSecret, address, code)
	message := email.Message{Kind: email.FirstTimeCode, Code: code, Link: deps.WebBaseURL + "/password-setup"}
	if recovery {
		user, err := deps.Store.GetUserByEmail(ctx, address)
		if errors.Is(err, store.ErrNotFound) {
			return "suppressed", nil
		}
		if err != nil {
			return "failed", err
		}
		if user.PasswordHash.Valid {
			kind = store.ResetLink
			token := auth.NewToken()
			digest, expires = auth.HashToken(token), deps.Now().Add(30*time.Minute)
			message = email.Message{Kind: email.ResetLink, Link: deps.WebBaseURL + "/reset-password#token=" + url.QueryEscape(token)}
		}
	}
	issued, err := deps.Store.IssuePasswordCredential(ctx, store.EmailCredentialParams{
		Email: address, Kind: kind, TokenHash: digest, ExpiresAt: expires,
	})
	if errors.Is(err, store.ErrNotFound) {
		return "suppressed", nil
	}
	if err != nil {
		return "failed", err
	}
	message.To, message.Name, message.ExpiresAt = issued.Email, issued.Name, expires
	if err := deps.Mailer.Send(ctx, message); err != nil {
		slog.WarnContext(ctx, "transactional email could not be sent", "kind", message.Kind)
		return "failed", nil
	}
	return "sent", nil
}
