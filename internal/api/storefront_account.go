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

// A Customer's password setup, recovery and profile (customers R7, R8, R11-R13, R15, R18).
// They follow the Users' (users-and-access R27-R31, R44) with Customer-only credentials, and
// their sends are counted apart from the Users' (storefrontSendScope).

// provinces are Thailand's 77 provinces (storefront R7), by their English names.
var provinces = []string{
	"Amnat Charoen", "Ang Thong", "Bangkok", "Bueng Kan", "Buri Ram", "Chachoengsao", "Chai Nat", "Chaiyaphum", "Chanthaburi",
	"Chiang Mai", "Chiang Rai", "Chon Buri", "Chumphon", "Kalasin", "Kamphaeng Phet", "Kanchanaburi", "Khon Kaen", "Krabi",
	"Lampang", "Lamphun", "Loei", "Lop Buri", "Mae Hong Son", "Maha Sarakham", "Mukdahan", "Nakhon Nayok", "Nakhon Pathom",
	"Nakhon Phanom", "Nakhon Ratchasima", "Nakhon Sawan", "Nakhon Si Thammarat", "Nan", "Narathiwat", "Nong Bua Lamphu",
	"Nong Khai", "Nonthaburi", "Pathum Thani", "Pattani", "Phang Nga", "Phatthalung", "Phayao", "Phetchabun", "Phetchaburi",
	"Phichit", "Phitsanulok", "Phra Nakhon Si Ayutthaya", "Phrae", "Phuket", "Prachin Buri", "Prachuap Khiri Khan", "Ranong",
	"Ratchaburi", "Rayong", "Roi Et", "Sa Kaeo", "Sakon Nakhon", "Samut Prakan", "Samut Sakhon", "Samut Songkhram", "Saraburi",
	"Satun", "Si Sa Ket", "Sing Buri", "Songkhla", "Sukhothai", "Suphan Buri", "Surat Thani", "Surin", "Tak", "Trang", "Trat",
	"Ubon Ratchathani", "Udon Thani", "Uthai Thani", "Uttaradit", "Yala", "Yasothon",
}

type deliveryAddress struct {
	Recipient   string `json:"recipient" maxLength:"120" pattern:"[^\\t-\\r \u0085   -     　]" patternDescription:"contain a character that is not whitespace" doc:"Who receives it, up to 120 characters."`
	Phone       string `json:"phone" pattern:"^0[0-9]{8,9}$" patternDescription:"be 9 or 10 digits starting with 0" doc:"The recipient's phone number: 9 or 10 digits, starting with 0 (customers R4)."`
	Line        string `json:"line" maxLength:"200" pattern:"[^\\t-\\r \u0085   -     　]" patternDescription:"contain a character that is not whitespace" doc:"The address line, up to 200 characters."`
	Subdistrict string `json:"subdistrict" maxLength:"80" pattern:"[^\\t-\\r \u0085   -     　]" patternDescription:"contain a character that is not whitespace" doc:"The sub-district, up to 80 characters."`
	District    string `json:"district" maxLength:"80" pattern:"[^\\t-\\r \u0085   -     　]" patternDescription:"contain a character that is not whitespace" doc:"The district, up to 80 characters."`
	Province    string `json:"province" doc:"One of Thailand's 77 provinces, by its English name."`
	Postcode    string `json:"postcode" pattern:"^[0-9]{5}$" patternDescription:"be 5 digits" doc:"The 5-digit postcode."`
}

// TransformSchema lists the provinces, which a struct tag cannot hold, so the schema refuses
// any other (storefront R7).
func (deliveryAddress) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	enum := make([]any, len(provinces))
	for i, p := range provinces {
		enum[i] = p
	}
	s.Properties["province"].Enum = enum
	return s
}

type updateCustomerInput struct {
	Session string `cookie:"customer_session" hidden:"true"`
	Body    struct {
		Name            *string          `json:"name,omitempty" maxLength:"120" pattern:"[^\\t-\\r \u0085   -     　]" patternDescription:"contain a character that is not whitespace" doc:"A new name, up to 120 characters. Leave out to keep it."`
		Phone           *string          `json:"phone,omitempty" pattern:"^0[0-9]{8,9}$" patternDescription:"be 9 or 10 digits starting with 0" doc:"A new phone number. Leave out to keep it."`
		DeliveryAddress *deliveryAddress `json:"deliveryAddress,omitempty" doc:"A new saved Delivery address, all of it. Leave out to keep it."`
	}
}

type setCustomerPasswordInput = emailPasswordInput

func registerStorefrontAccount(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "request-customer-password-setup", Method: http.MethodPost, Path: "/v2/storefront/password-setup/request",
		Summary: "Email a Verification code to a Customer who has no password",
		Description: "Always answers the same 202 message: whether the email is a Customer's, Inactive, has a password already, is limited or failed to send. " +
			"Sends are limited as sign-up's are. Answers 503 `email-auth-unavailable` when email is not configured.",
		Tags: []string{"Storefront account"}, DefaultStatus: http.StatusAccepted,
		Errors: []int{http.StatusInternalServerError, http.StatusServiceUnavailable},
	}, func(ctx context.Context, input *emailRequestInput) (*emailRequestOutput, error) {
		return requestCustomerInstructions(ctx, deps, input.Body.Email, false)
	})

	huma.Register(api, huma.Operation{
		OperationID: "verify-customer-password-setup-code", Method: http.MethodPost, Path: "/v2/storefront/password-setup/verify",
		Summary: "Exchange a Customer's Verification code for a password-setup grant",
		Description: "Answers 410 `signup-code-invalid` for a wrong, expired, used, replaced, blocked or ineligible code: five wrong attempts block codes for the email for 10 minutes. " +
			"Answers 503 `email-auth-unavailable` when not configured. No session starts.",
		Tags:   []string{"Storefront account"},
		Errors: []int{http.StatusGone, http.StatusInternalServerError, http.StatusServiceUnavailable},
	}, func(ctx context.Context, input *verifySignupCodeInput) (*verifySignupCodeOutput, error) {
		if err := emailAuthReady(deps); err != nil {
			return nil, err
		}
		now := deps.Now()
		token := auth.NewToken()
		expires := now.Add(signupGrantLifetime)
		verified, err := deps.Store.VerifyCustomerCode(ctx, store.VerifyFirstTimeCodeParams{
			Email: input.Body.Email, CodeHash: auth.HashCustomerCode(deps.CodeSecret, input.Body.Email, input.Body.Code),
			GrantHash: auth.HashToken(token), GrantExpiresAt: expires, At: now,
		})
		if err != nil {
			return nil, internalError(ctx, "verify Customer code", err)
		}
		if !verified {
			return nil, domainError(http.StatusGone, codeSignupCodeInvalid, "This code cannot be used. Request a new one or try again later.")
		}
		out := &verifySignupCodeOutput{}
		out.Body.Token, out.Body.ExpiresAt = token, expires
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "set-customer-password", Method: http.MethodPost, Path: "/v2/storefront/password-setup/password",
		Summary: "Set a Customer's first password using a verified grant",
		Description: "Answers 410 `customer-password-setup-invalid` for an expired, used, replaced or ineligible grant, and 503 `email-auth-unavailable` when not configured. " +
			"Starts no session: log in next.",
		Tags: []string{"Storefront account"}, DefaultStatus: http.StatusNoContent,
		Errors: []int{http.StatusGone, http.StatusInternalServerError, http.StatusServiceUnavailable},
	}, func(ctx context.Context, input *setCustomerPasswordInput) (*struct{}, error) {
		_, err := saveCustomerPassword(ctx, deps, input, store.SetupGrant)
		return nil, err
	})

	huma.Register(api, huma.Operation{
		OperationID: "request-customer-password-reset", Method: http.MethodPost, Path: "/v2/storefront/password-reset/request",
		Summary: "Email a Customer a Reset link (Forgot password)",
		Description: "Always answers the same 202 message: for an unknown, Inactive, limited or failed-to-send email alike. A Customer without a password is sent a Verification code instead. " +
			"Not blocked by failed logins. Answers 503 `email-auth-unavailable` when email is not configured.",
		Tags: []string{"Storefront account"}, DefaultStatus: http.StatusAccepted,
		Errors: []int{http.StatusInternalServerError, http.StatusServiceUnavailable},
	}, func(ctx context.Context, input *emailRequestInput) (*emailRequestOutput, error) {
		return requestCustomerInstructions(ctx, deps, input.Body.Email, true)
	})

	huma.Register(api, huma.Operation{
		OperationID: "reset-customer-password", Method: http.MethodPost, Path: "/v2/storefront/password-reset/password",
		Summary: "Set a Customer's new password using an emailed Reset link",
		Description: "The link works once, for 30 minutes, and only the newest works. Ends all the Customer's sessions and starts none, and emails them that the password changed. " +
			"Answers 410 `customer-password-reset-invalid` for an expired, used, replaced or ineligible link, and 503 `email-auth-unavailable` when not configured.",
		Tags: []string{"Storefront account"}, DefaultStatus: http.StatusNoContent,
		Errors: []int{http.StatusGone, http.StatusInternalServerError, http.StatusServiceUnavailable},
	}, func(ctx context.Context, input *setCustomerPasswordInput) (*struct{}, error) {
		updated, err := saveCustomerPassword(ctx, deps, input, store.ResetLink)
		if err != nil {
			return nil, err
		}
		// customers R12, R18: the change is made whether or not the notice goes.
		if err := deps.Mailer.Send(ctx, email.Message{Kind: email.ResetComplete, To: updated.Email, Name: updated.Name, Link: deps.WebBaseURL + "/customer/login"}); err != nil {
			slog.WarnContext(ctx, "password changed notification could not be sent")
		}
		return nil, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-current-customer", Method: http.MethodPatch, Path: "/v2/storefront/me",
		Summary: "Change the logged-in Customer's name, phone number or saved Delivery address",
		Description: "Changes only what the body names. The email cannot be changed, and neither can the password while logged in: use Forgot password. " +
			"Each change is recorded in the Activity log with its value before and after. " + customerSessionErrors,
		Tags: []string{"Storefront sessions"}, Security: customerSessionSecurity,
		Errors: []int{http.StatusUnauthorized, http.StatusInternalServerError},
	}, func(ctx context.Context, input *updateCustomerInput) (*currentCustomerOutput, error) {
		session, err := liveCustomerSession(ctx, deps, input.Session)
		if err != nil {
			return nil, err
		}
		arg := store.ChangeProfileParams{ID: session.CustomerID, Name: input.Body.Name, Phone: input.Body.Phone}
		if a := input.Body.DeliveryAddress; a != nil {
			arg.Address = &store.DeliveryAddress{
				Recipient: a.Recipient, Phone: a.Phone, Line: a.Line, Subdistrict: a.Subdistrict, District: a.District, Province: a.Province, Postcode: a.Postcode,
			}
		}
		updated, err := deps.Store.UpdateProfile(ctx, arg, requestBy(ctx, deps))
		if errors.Is(err, store.ErrNotFound) {
			return nil, domainError(http.StatusUnauthorized, codeSessionEnded, "Your session has ended. Log in again.")
		}
		if err != nil {
			return nil, internalError(ctx, "update Customer profile", err)
		}
		return &currentCustomerOutput{Body: customerBody(updated)}, nil
	})
}

func saveCustomerPassword(ctx context.Context, deps Deps, input *setCustomerPasswordInput, kind string) (store.Customer, error) {
	if err := emailAuthReady(deps); err != nil {
		return store.Customer{}, err
	}
	code, detail := codeCustomerPasswordSetupInvalid, "This password-setup grant no longer works. Request a new code."
	if kind == store.ResetLink {
		code, detail = codeCustomerPasswordResetInvalid, "This Reset link no longer works. Request a new one through Forgot password."
	}
	refusal := func(doing string, err error) error {
		if errors.Is(err, store.ErrNotFound) {
			return domainError(http.StatusGone, code, detail)
		}
		return internalError(ctx, doing, err)
	}
	arg := store.EmailPasswordParams{Kind: kind, TokenHash: auth.HashToken(input.Body.Token), At: deps.Now()}
	// Before the password is hashed, so a made-up token costs nothing.
	if err := deps.Store.ReadCustomerCredential(ctx, arg); err != nil {
		return store.Customer{}, refusal("read Customer credential", err)
	}
	arg.PasswordHash = deps.PasswordHasher.Hash(input.Body.Password)
	arg.At = deps.Now()
	updated, err := deps.Store.SetCustomerPassword(ctx, arg, requestBy(ctx, deps))
	if err != nil {
		return store.Customer{}, refusal("save Customer password", err)
	}
	return updated, nil
}

func requestCustomerInstructions(ctx context.Context, deps Deps, address string, recovery bool) (*emailRequestOutput, error) {
	if err := emailAuthReady(deps); err != nil {
		return nil, err
	}
	if err := sendCustomerInstructions(ctx, deps, address, recovery); err != nil {
		return nil, internalError(ctx, "request Customer instructions", err)
	}
	out := &emailRequestOutput{}
	out.Body.Message = emailRequestMessage
	return out, nil
}

// sendCustomerInstructions emails a Customer a Verification code, or on recovery a Reset link
// when they have a password, and says nothing of how it went to anyone (customers R7, R12;
// users-and-access R44). Unknown and Inactive Customers receive nothing (customers R15).
func sendCustomerInstructions(ctx context.Context, deps Deps, address string, recovery bool) error {
	info, _ := ctx.Value(requestInfoKey{}).(requestInfo)
	allowed, err := deps.Store.ReserveEmailRequest(ctx, store.EmailRequestParams{
		EmailHash: auth.HashToken(storefrontSendScope + strings.ToLower(address)), IPHash: auth.HashToken(storefrontSendScope + info.ip), At: deps.Now(),
	})
	if err != nil || !allowed {
		return err
	}
	c, err := deps.Store.GetCustomerByEmail(ctx, address)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	kind := store.FirstTimeCode
	code := auth.NewFirstTimeCode()
	digest := auth.HashCustomerCode(deps.CodeSecret, address, code)
	expires := deps.Now().Add(signupCodeLifetime)
	message := email.Message{Kind: email.VerificationCode, Code: code, Link: deps.WebBaseURL + "/customer/password-setup"}
	if recovery && c.PasswordHash.Valid {
		kind = store.ResetLink
		token := auth.NewToken()
		digest, expires = auth.HashToken(token), deps.Now().Add(resetLinkLifetime)
		message = email.Message{Kind: email.ResetLink, Link: deps.WebBaseURL + "/customer/reset-password#token=" + url.QueryEscape(token)}
	}
	issued, err := deps.Store.IssueCustomerCredential(ctx, store.EmailCredentialParams{Email: address, Kind: kind, TokenHash: digest, ExpiresAt: expires})
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	message.To, message.Name, message.ExpiresAt = issued.Email, issued.Name, expires
	if err := deps.Mailer.Send(ctx, message); err != nil {
		slog.WarnContext(ctx, "transactional email could not be sent", "kind", message.Kind)
	}
	return nil
}

// resetLinkLifetime is customers R12's 30 minutes.
const resetLinkLifetime = 30 * time.Minute
