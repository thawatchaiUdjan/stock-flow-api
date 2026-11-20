package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/email"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// Customers sign up, log in and log out on the Storefront (customers R1-R6, R8-R11; business
// ADR 0014). A Customer's session is its own table and its own cookie, so a StockFlow
// operation, which reads only the User's, never sees it, and a Storefront operation never
// sees a User's (customers R1).

const (
	// customerSessionCookieName also names the contract's security scheme. Keep it in step
	// with customerSessionInput's tag, which can only be a literal.
	customerSessionCookieName = "customer_session"

	// signupCodeLifetime and signupGrantLifetime are customers R8's ten minutes, and the
	// time a verified email has to finish signing up.
	signupCodeLifetime  = 10 * time.Minute
	signupGrantLifetime = 10 * time.Minute

	// storefrontSendScope keeps the Verification code send limits of customers R8 apart from
	// the Users' (users-and-access R43): the same ledger, other keys.
	storefrontSendScope = "storefront:"
)

var customerSessionSecurity = []map[string][]string{{customerSessionCookieName: {}}}

// customerSessionErrors is how every Customer operation that needs a session describes its 401s.
const customerSessionErrors = "Answers a plain 401 with no `customer_session` cookie, and 401 `session-ended` " +
	"with a cookie that matches no live Customer session."

// customerSessionInput is the input of an operation that needs a live Customer session. The
// cookie is hidden because the browser sends it, not the generated client.
type customerSessionInput struct {
	Session string `cookie:"customer_session" hidden:"true"`
}

type currentCustomer struct {
	ID    int64  `json:"id" doc:"Identifies the Customer."`
	Name  string `json:"name" doc:"The Customer's name, in any language."`
	Email string `json:"email" doc:"The Customer's email, in lower case."`
	Phone string `json:"phone" doc:"The Customer's phone number: 9 or 10 digits, starting with 0."`
}

type requestSignupCodeInput struct {
	Body struct {
		Email string `json:"email" format:"email" doc:"Compared without regard to letter case."`
	}
}

type requestSignupCodeOutput struct {
	Body struct {
		CodeDelivery string `json:"codeDelivery" enum:"sent,failed,suppressed" doc:"sent means provider acceptance, not inbox delivery; suppressed means no new code went out because of the send limits; failed means the provider refused. A previous unexpired code keeps working."`
	}
}

type verifySignupCodeInput struct {
	Body struct {
		Email string `json:"email" format:"email"`
		Code  string `json:"code" pattern:"^[0-9]{6}$" minLength:"6" maxLength:"6"`
	}
}

type verifySignupCodeOutput struct {
	Body struct {
		Token     string    `json:"token" doc:"Single-use sign-up grant. Keep out of paths, query strings and logs."`
		ExpiresAt time.Time `json:"expiresAt"`
	}
}

type completeSignupInput struct {
	Body struct {
		Token string `json:"token" doc:"The sign-up grant a verified code was exchanged for."`
		// customers R2: required, whatever the language; the pattern is any non-whitespace.
		Name string `json:"name" maxLength:"120" pattern:"[^\\t-\\r \u0085   -     　]" patternDescription:"contain a character that is not whitespace" doc:"The Customer's name, up to 120 characters, in any language."`
		// customers R4.
		Phone string `json:"phone" pattern:"^0[0-9]{8,9}$" patternDescription:"be 9 or 10 digits starting with 0" doc:"9 or 10 digits, starting with 0."`
		// customers R11, business ADR 0017: 6 to 128 characters.
		Password string `json:"password" minLength:"6" maxLength:"128"`
	}
}

type completeSignupOutput struct {
	Body currentCustomer
}

type customerLogInInput struct {
	Body struct {
		// No format or length here: an email or password that couldn't be right must fail
		// like any other (customers R9). Only a body missing a field gets a 422.
		Email    string `json:"email" doc:"Compared without regard to letter case."`
		Password string `json:"password"`
	}
}

type customerLogInOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Starts the Customer session."`
	Body      currentCustomer
}

type customerLogOutOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Removes the Customer session cookie."`
}

type currentCustomerOutput struct {
	Body currentCustomer
}

func registerStorefrontAuth(api huma.API, deps Deps) {
	components := api.OpenAPI().Components
	if components.SecuritySchemes == nil {
		components.SecuritySchemes = map[string]*huma.SecurityScheme{}
	}
	components.SecuritySchemes[customerSessionCookieName] = &huma.SecurityScheme{
		Type: "apiKey", In: "cookie", Name: customerSessionCookieName,
	}

	huma.Register(api, huma.Operation{
		OperationID: "request-signup-code", Method: http.MethodPost, Path: "/v2/storefront/signup/request",
		Summary: "Email a Verification code to start signing up",
		Description: "Nothing exists until the sign-up is completed. Answers 409 `customer-email-taken` when a Customer has the email, and sends nothing. " +
			"Sends are limited as the Users' are (one per 60 seconds, five per email and 30 per IP address per hour), counted separately for Customers; " +
			"a limited request still answers 202, with `codeDelivery` suppressed. Answers 503 `email-auth-unavailable` when email is not configured.",
		Tags: []string{"Storefront sign-up"}, DefaultStatus: http.StatusAccepted,
		Errors: []int{http.StatusConflict, http.StatusInternalServerError, http.StatusServiceUnavailable},
	}, func(ctx context.Context, input *requestSignupCodeInput) (*requestSignupCodeOutput, error) {
		if err := emailAuthReady(deps); err != nil {
			return nil, err
		}
		delivery, err := sendVerificationCode(ctx, deps, input.Body.Email)
		if err != nil {
			return nil, err
		}
		out := &requestSignupCodeOutput{}
		out.Body.CodeDelivery = delivery
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "verify-signup-code", Method: http.MethodPost, Path: "/v2/storefront/signup/verify",
		Summary: "Exchange a Verification code for a sign-up grant",
		Description: "Answers 410 `signup-code-invalid` for a wrong, expired, used, replaced or blocked code: five wrong attempts block codes for the email for 10 minutes. " +
			"Answers 503 `email-auth-unavailable` when email is not configured. No Customer exists yet and no session starts.",
		Tags:   []string{"Storefront sign-up"},
		Errors: []int{http.StatusGone, http.StatusInternalServerError, http.StatusServiceUnavailable},
	}, func(ctx context.Context, input *verifySignupCodeInput) (*verifySignupCodeOutput, error) {
		if err := emailAuthReady(deps); err != nil {
			return nil, err
		}
		now := deps.Now()
		token := auth.NewToken()
		expires := now.Add(signupGrantLifetime)
		verified, err := deps.Store.VerifySignupCode(ctx, store.VerifyFirstTimeCodeParams{
			Email: input.Body.Email, CodeHash: auth.HashSignupCode(deps.CodeSecret, input.Body.Email, input.Body.Code),
			GrantHash: auth.HashToken(token), GrantExpiresAt: expires, At: now,
		})
		if err != nil {
			return nil, internalError(ctx, "verify sign-up code", err)
		}
		if !verified {
			return nil, domainError(http.StatusGone, codeSignupCodeInvalid, "This code cannot be used. Request a new one or try again later.")
		}
		out := &verifySignupCodeOutput{}
		out.Body.Token, out.Body.ExpiresAt = token, expires
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "complete-signup", Method: http.MethodPost, Path: "/v2/storefront/signup/complete",
		Summary: "Finish signing up with a name, a phone number and a password",
		Description: "Creates the Customer, Active, and records the sign-up in the Activity log with the Customer as the actor. Starts no session: log in next. " +
			"Answers 410 `signup-invalid` for an expired or used grant, 409 `customer-email-taken` when a Customer has the email by now, and 503 `email-auth-unavailable` when email is not configured.",
		Tags: []string{"Storefront sign-up"}, DefaultStatus: http.StatusCreated,
		Errors: []int{http.StatusConflict, http.StatusGone, http.StatusInternalServerError, http.StatusServiceUnavailable},
	}, func(ctx context.Context, input *completeSignupInput) (*completeSignupOutput, error) {
		if err := emailAuthReady(deps); err != nil {
			return nil, err
		}
		invalid := domainError(http.StatusGone, codeSignupInvalid, "This sign-up can no longer be completed. Verify a new code.")
		arg := store.CompleteSignupParams{GrantHash: auth.HashToken(input.Body.Token), At: deps.Now(), Name: input.Body.Name, Phone: input.Body.Phone}
		// Before the password is hashed, so a made-up grant costs nothing.
		if err := deps.Store.ReadSignupGrant(ctx, store.SignupGrantParams{TokenHash: arg.GrantHash, At: arg.At}); errors.Is(err, store.ErrNotFound) {
			return nil, invalid
		} else if err != nil {
			return nil, internalError(ctx, "complete sign-up: read grant", err)
		}
		arg.PasswordHash = deps.PasswordHasher.Hash(input.Body.Password)
		arg.At = deps.Now()
		created, err := deps.Store.CompleteSignup(ctx, arg, requestBy(ctx, deps))
		switch {
		case errors.Is(err, store.ErrNotFound):
			return nil, invalid
		case errors.Is(err, store.ErrCustomerEmailTaken):
			return nil, domainError(http.StatusConflict, codeCustomerEmailTaken, "A Customer with this email already exists.")
		case err != nil:
			return nil, internalError(ctx, "complete sign-up", err)
		}
		return &completeSignupOutput{Body: customerBody(created.ID, created.Name, created.Email, created.Phone)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-customer-session", Method: http.MethodPost, Path: "/v2/storefront/login",
		Summary: "Log a Customer in with an email and a password",
		Description: "Starts a Customer session, which reaches only the Storefront. Every failure answers the same 401 `login-failed`, in the same time: a wrong email, a wrong password, an Inactive Customer, or a Customer with no password. " +
			"After 5 failed logins for an email within 15 minutes, or 50 from an IP address within an hour, on the Storefront, answers 429 `login-blocked` with Retry-After, " +
			"and records a `blocked` Login log entry. The Storefront's failures are counted apart from StockFlow's.",
		Tags: []string{"Storefront sessions"}, Errors: []int{http.StatusUnauthorized, http.StatusTooManyRequests},
	}, func(ctx context.Context, input *customerLogInInput) (*customerLogInOutput, error) {
		failed := domainError(http.StatusUnauthorized, codeLoginFailed, "Email or password is incorrect.")
		customer, err := deps.Store.GetCustomerByEmail(ctx, input.Body.Email)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, internalError(ctx, "customer log in: get Customer", err)
		}
		// users-and-access R36: the email as typed, and the Customer only if it matches one.
		entry := customerLoginLogEntry(ctx, deps, store.ResultFailedLogin, input.Body.Email, customer.ID)
		// users-and-access R55-R57: before the password is looked at, and the same answer
		// for every email.
		blockedUntil, err := deps.Store.LoginBlockedUntil(ctx, store.LoginBlockParams{App: store.AppStorefront, Email: input.Body.Email, IP: entry.Ip, At: entry.At})
		if err != nil {
			return nil, internalError(ctx, "customer log in: check login limits", err)
		}
		if !blockedUntil.IsZero() {
			entry.Result = store.ResultBlocked
			if err := deps.Store.AddLoginLogEntry(ctx, entry); err != nil {
				return nil, internalError(ctx, "customer log in: add login log entry", err)
			}
			return nil, retryLater(codeLoginBlocked, "Logins are temporarily blocked. Try again later.", blockedUntil, entry.At)
		}
		// Verify runs on every path and before the status is looked at, so an unknown email,
		// a Customer with no password and an Inactive Customer all take as long as a wrong
		// password (customers R9). The zero Customer has an empty hash.
		verified := deps.PasswordHasher.Verify(customer.PasswordHash.String, input.Body.Password)
		if !verified || customer.Status != statusActive {
			if err := deps.Store.AddLoginLogEntry(ctx, entry); err != nil {
				return nil, internalError(ctx, "customer log in: add login log entry", err)
			}
			return nil, failed
		}

		token := auth.NewToken()
		started, err := deps.Store.StartCustomerSession(ctx, store.StartCustomerSessionParams{
			Session: store.CreateCustomerSessionParams{
				TokenHash: auth.HashToken(token), CustomerID: customer.ID, ExpiresAt: deps.Now().Add(sessionLifetime), PasswordHash: customer.PasswordHash,
			},
			Entry: entry,
		})
		if err != nil {
			return nil, internalError(ctx, "customer log in: start session", err)
		}
		// The Customer changed while Verify ran; StartCustomerSession has logged a failed login.
		if !started {
			return nil, failed
		}
		out := &customerLogInOutput{SetCookie: customerSessionCookie(token, cookieLifetime)}
		out.Body = customerBody(customer.ID, customer.Name, customer.Email, customer.Phone)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "end-customer-session", Method: http.MethodPost, Path: "/v2/storefront/logout",
		Summary:     "End this device's Customer session",
		Description: "The Customer's sessions on other devices stay live. " + customerSessionErrors,
		Tags:        []string{"Storefront sessions"}, Security: customerSessionSecurity, Errors: []int{http.StatusUnauthorized},
	}, func(ctx context.Context, input *customerSessionInput) (*customerLogOutOutput, error) {
		session, err := liveCustomerSession(ctx, deps, input.Session)
		if err != nil {
			return nil, err
		}
		err = deps.Store.EndCustomerSession(ctx, store.EndCustomerSessionParams{
			TokenHash: auth.HashToken(input.Session),
			Entry:     customerLoginLogEntry(ctx, deps, store.ResultLogout, session.Email, session.CustomerID),
		})
		if err != nil {
			return nil, internalError(ctx, "customer log out: end session", err)
		}
		return &customerLogOutOutput{SetCookie: customerSessionCookie("", -time.Second)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-current-customer", Method: http.MethodGet, Path: "/v2/storefront/me",
		Summary: "Get the Customer who is logged in", Description: customerSessionErrors,
		Tags: []string{"Storefront sessions"}, Security: customerSessionSecurity, Errors: []int{http.StatusUnauthorized},
	}, func(ctx context.Context, input *customerSessionInput) (*currentCustomerOutput, error) {
		session, err := liveCustomerSession(ctx, deps, input.Session)
		if err != nil {
			return nil, err
		}
		return &currentCustomerOutput{Body: customerBody(session.CustomerID, session.Name, session.Email, session.Phone)}, nil
	})
}

func customerBody(id int64, name, emailAddress, phone string) currentCustomer {
	return currentCustomer{ID: id, Name: name, Email: emailAddress, Phone: phone}
}

// sendVerificationCode emails a sign-up code and says how it went (customers R5, R6, R8).
func sendVerificationCode(ctx context.Context, deps Deps, address string) (string, error) {
	info, _ := ctx.Value(requestInfoKey{}).(requestInfo)
	// The limits come first, so asking about emails is as limited as sending to them.
	allowed, err := deps.Store.ReserveEmailRequest(ctx, store.EmailRequestParams{
		EmailHash: auth.HashToken(storefrontSendScope + strings.ToLower(address)), IPHash: auth.HashToken(storefrontSendScope + info.ip), At: deps.Now(),
	})
	if err != nil {
		return "", internalError(ctx, "request sign-up code: reserve", err)
	}
	if !allowed {
		return "suppressed", nil
	}
	code := auth.NewFirstTimeCode()
	expires := deps.Now().Add(signupCodeLifetime)
	err = deps.Store.IssueSignupCode(ctx, store.SignupCodeParams{Email: address, CodeHash: auth.HashSignupCode(deps.CodeSecret, address, code), ExpiresAt: expires})
	// customers R6: the store looks under the email's lock, so a Customer made meanwhile is not missed.
	if errors.Is(err, store.ErrCustomerEmailTaken) {
		return "", domainError(http.StatusConflict, codeCustomerEmailTaken, "A Customer with this email already exists. Log in, or use Forgot password.")
	}
	if err != nil {
		return "", internalError(ctx, "request sign-up code: issue", err)
	}
	err = deps.Mailer.Send(ctx, email.Message{Kind: email.VerificationCode, To: address, Code: code, Link: deps.WebBaseURL + "/signup", ExpiresAt: expires})
	if err != nil {
		slog.WarnContext(ctx, "transactional email could not be sent", "kind", email.VerificationCode)
		return "failed", nil
	}
	return "sent", nil
}

// liveCustomerSession returns the Customer session the request carries, as liveSession does
// for a User's (customers R10).
func liveCustomerSession(ctx context.Context, deps Deps, cookie string) (store.GetCustomerSessionRow, error) {
	if cookie == "" {
		return store.GetCustomerSessionRow{}, huma.Error401Unauthorized("Log in to continue.")
	}
	session, err := deps.Store.GetCustomerSession(ctx, auth.HashToken(cookie))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return store.GetCustomerSessionRow{}, internalError(ctx, "get Customer session", err)
	}
	// customers R10: the expiry was fixed at login. A row not found leaves the zero time,
	// which is long past.
	if !deps.Now().Before(session.ExpiresAt) {
		return store.GetCustomerSessionRow{}, domainError(http.StatusUnauthorized, codeSessionEnded, "Your session has ended. Log in again.")
	}
	return session, nil
}

// customerSessionCookie is sessionCookie under the Customer's name.
func customerSessionCookie(value string, lifetime time.Duration) http.Cookie {
	cookie := sessionCookie(value, lifetime)
	cookie.Name = customerSessionCookieName
	return cookie
}
