package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

const (
	// sessionCookieName also names the contract's security scheme. Keep it in step with
	// sessionInput's tag, which can only be a literal.
	sessionCookieName = "session"

	// sessionLifetime is fixed at login and never extended (users-and-access R21).
	sessionLifetime = 7 * 24 * time.Hour

	// cookieLifetime outlasts the session, so a request after the session ended still
	// carries the cookie and hears why it was refused (users-and-access R26). 400 days is
	// the most a browser keeps a cookie; all it holds by then is a dead token.
	cookieLifetime = 400 * 24 * time.Hour
)

// statusActive is the status of a User who may log in.
const statusActive = "active"

// sessionInput is the input of an operation that needs a live session. The cookie is
// hidden because the browser sends it, not the generated client: the contract declares
// it as a security scheme instead.
type sessionInput struct {
	Session string `cookie:"session" hidden:"true"`
}

// sessionSecurity marks an operation as needing a live session in the contract.
var sessionSecurity = []map[string][]string{{sessionCookieName: {}}}

// sessionErrors is how every operation that needs a session describes its 401s.
const sessionErrors = "Answers a plain 401 with no session cookie, and 401 `session-ended` " +
	"with a cookie that matches no live session."

type currentUser struct {
	Name  string `json:"name" doc:"The User's name, in any language."`
	Email string `json:"email" doc:"The User's email, as it was entered when the User was created."`
	Role  string `json:"role" enum:"owner,admin,staff" doc:"The User's Role."`
}

type logInInput struct {
	Body struct {
		// No format or length here: an email or password that couldn't be right must fail
		// like any other, not with a different answer (users-and-access R5). Only a body
		// missing a field gets a 422, which depends on no User and so tells nothing.
		Email    string `json:"email" doc:"Compared without regard to letter case."`
		Password string `json:"password"`
	}
}

type logInOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Starts the session."`
	Body      currentUser
}

type logOutOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Removes the session cookie."`
}

type currentUserOutput struct {
	Body currentUser
}

func registerSessions(api huma.API, deps Deps) {
	components := api.OpenAPI().Components
	if components.SecuritySchemes == nil {
		components.SecuritySchemes = map[string]*huma.SecurityScheme{}
	}
	components.SecuritySchemes[sessionCookieName] = &huma.SecurityScheme{
		Type: "apiKey",
		In:   "cookie",
		Name: sessionCookieName,
	}

	huma.Register(api, huma.Operation{
		OperationID: "create-session",
		Method:      http.MethodPost,
		Path:        "/login",
		Summary:     "Log in with an email and a password",
		Description: "Starts a session. Every failure answers the same 401 `login-failed`, in the same time.",
		Tags:        []string{"Sessions"},
		Errors:      []int{http.StatusUnauthorized},
	}, func(ctx context.Context, input *logInInput) (*logInOutput, error) {
		failed := domainError(http.StatusUnauthorized, codeLoginFailed, "Email or password is incorrect.")
		user, err := deps.Store.GetUserByEmail(ctx, input.Body.Email)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, internalError(ctx, "log in: get user", err)
		}
		// users-and-access R36: the email as typed, and the User only if it matches one. An
		// unknown email leaves the zero User, whose ID is 0.
		entry := loginLogEntry(ctx, deps, store.ResultFailedLogin, input.Body.Email, user.ID)
		// Verify runs on every path and before the status is looked at, so an unknown
		// email, a User with no password and an Inactive User all take as long as a wrong
		// password (users-and-access R5, R16). The zero User has an empty hash.
		verified := deps.PasswordHasher.Verify(user.PasswordHash.String, input.Body.Password)
		if !verified || user.Status != statusActive {
			if err := deps.Store.AddLoginLogEntry(ctx, entry); err != nil {
				return nil, internalError(ctx, "log in: add login log entry", err)
			}
			return nil, failed
		}

		token := auth.NewToken()
		started, err := deps.Store.StartSession(ctx, store.StartSessionParams{
			Session: store.CreateSessionParams{
				TokenHash:    auth.HashToken(token),
				UserID:       user.ID,
				ExpiresAt:    deps.Now().Add(sessionLifetime),
				PasswordHash: user.PasswordHash,
			},
			Entry: entry,
		})
		if err != nil {
			return nil, internalError(ctx, "log in: start session", err)
		}
		// The User was set Inactive or reset, or the Owner's password was set, while Verify
		// ran (users-and-access R16, R31, R34); StartSession has logged it as a failed login.
		if !started {
			return nil, failed
		}
		out := &logInOutput{SetCookie: sessionCookie(token, cookieLifetime)}
		out.Body = currentUser{Name: user.Name, Email: user.Email, Role: user.Role}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "end-session",
		Method:      http.MethodPost,
		Path:        "/logout",
		Summary:     "End this device's session",
		Description: "The User's sessions on other devices stay live. " + sessionErrors,
		Tags:        []string{"Sessions"},
		Security:    sessionSecurity,
		Errors:      []int{http.StatusUnauthorized},
	}, func(ctx context.Context, input *sessionInput) (*logOutOutput, error) {
		session, err := liveSession(ctx, deps, input.Session)
		if err != nil {
			return nil, err
		}
		err = deps.Store.EndSession(ctx, store.EndSessionParams{
			TokenHash: auth.HashToken(input.Session),
			Entry:     loginLogEntry(ctx, deps, store.ResultLogout, session.Email, session.UserID),
		})
		if err != nil {
			return nil, internalError(ctx, "log out: end session", err)
		}
		// users-and-access R7. The cookie goes too: having logged out is not a session
		// that ended on its own, so the next request must not hear session-ended.
		return &logOutOutput{SetCookie: sessionCookie("", -time.Second)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-current-user",
		Method:      http.MethodGet,
		Path:        "/me",
		Summary:     "Get the User who is logged in",
		Description: sessionErrors,
		Tags:        []string{"Sessions"},
		Security:    sessionSecurity,
		Errors:      []int{http.StatusUnauthorized},
	}, func(ctx context.Context, input *sessionInput) (*currentUserOutput, error) {
		session, err := liveSession(ctx, deps, input.Session)
		if err != nil {
			return nil, err
		}
		out := &currentUserOutput{}
		out.Body = currentUser{Name: session.Name, Email: session.Email, Role: session.Role}
		return out, nil
	})
}

// liveSession returns the session the request carries. With no cookie it answers the
// plain 401 of a visitor; with a cookie that matches no live session it answers
// session-ended, so the web app can tell the two apart (users-and-access R26).
func liveSession(ctx context.Context, deps Deps, cookie string) (store.GetSessionRow, error) {
	if cookie == "" {
		return store.GetSessionRow{}, huma.Error401Unauthorized("Log in to continue.")
	}
	session, err := deps.Store.GetSession(ctx, auth.HashToken(cookie))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return store.GetSessionRow{}, internalError(ctx, "get session", err)
	}
	// users-and-access R21: the expiry was fixed at login. A row not found leaves the
	// zero time, which is long past.
	if !deps.Now().Before(session.ExpiresAt) {
		return store.GetSessionRow{}, domainError(http.StatusUnauthorized, codeSessionEnded, "Your session has ended. Log in again.")
	}
	return session, nil
}

// sessionCookie is the cookie of ADR 0015. A negative lifetime removes it.
func sessionCookie(value string, lifetime time.Duration) http.Cookie {
	return http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   int(lifetime.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}
