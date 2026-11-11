package api

import (
	"context"
	"log/slog"
	"net/http"
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// The domain error codes, carried in Problem Details' type (ADR 0019). They are a
// contract: stock-flow-web checks against them, so renaming one breaks it.
const (
	codeLoginFailed          = "login-failed"
	codeSessionEnded         = "session-ended"
	codeSetupLinkInvalid     = "setup-link-invalid"
	codeEmailTaken           = "email-taken"
	codeTargetIsOwner        = "target-is-owner"
	codeUserInactive         = "user-inactive"
	codeSKUTaken             = "sku-taken"
	codeInsufficientStock    = "insufficient-stock"
	codeProductArchived      = "product-archived"
	codeOrderNotPending      = "order-not-pending"
	codeEmailAuthUnavailable = "email-auth-unavailable"
	codeFirstTimeCodeInvalid = "first-time-code-invalid"
	codePasswordSetupInvalid = "password-setup-invalid"
	codePasswordResetInvalid = "password-reset-invalid"
)

// errorCodes is the published list. Every code above must be in it.
var errorCodes = []struct{ code, meaning string }{
	{codeLoginFailed, "The email and password did not log anyone in. Identical for every cause."},
	{codeSessionEnded, "The request carried a session cookie that matches no live session."},
	{codeSetupLinkInvalid, "The Setup link is used, expired, replaced or was never issued."},
	{codeEmailTaken, "A User has this email already, in any letter case."},
	{codeTargetIsOwner, "The operation acts on a User, and that User is the Owner, who cannot be changed here."},
	{codeUserInactive, "No Setup link can be issued for an Inactive User."},
	{codeSKUTaken, "Another Product has this SKU, in any letter case."},
	{codeInsufficientStock, "The stock change, or the Order, would take a Product's stock below 0. The error's products member names each such Product with its stock."},
	{codeProductArchived, "The Product is Archived: its stock cannot be received or adjusted, and it cannot be ordered."},
	{codeOrderNotPending, "The Order is completed or cancelled already, and neither can change."},
	{codeEmailAuthUnavailable, "Email password setup and recovery are not configured."},
	{codeFirstTimeCodeInvalid, "The First-time code cannot be used: it is wrong, expired, consumed, replaced, blocked or ineligible."},
	{codePasswordSetupInvalid, "The password-setup grant is expired, consumed, replaced or ineligible."},
	{codePasswordResetInvalid, "The Reset link is expired, consumed, replaced or ineligible."},
}

func domainError(status int, code, detail string) *huma.ErrorModel {
	return &huma.ErrorModel{Type: code, Title: http.StatusText(status), Status: status, Detail: detail}
}

// productStock is a Product named by an insufficient-stock error, with its stock.
type productStock struct {
	ID    int64 `json:"id" doc:"Identifies the Product."`
	Stock int32 `json:"stock" doc:"The Product's stock: how many units there are."`
}

// stockProblem is the insufficient-stock error. It names every Product that falls short,
// with its stock, so the web can say "Only 18 in stock." beside each (stock-movements R6,
// orders R8). RFC 9457 lets a problem carry members of its own.
type stockProblem struct {
	huma.ErrorModel
	Products []productStock `json:"products"`
}

// internalError logs what went wrong and answers without it (CODING_STANDARDS.md, Errors).
func internalError(ctx context.Context, doing string, err error) huma.StatusError {
	slog.ErrorContext(ctx, doing+" failed", "err", err)
	return huma.Error500InternalServerError("something went wrong on our side")
}

// publishErrorCodes narrows ErrorModel's type to the list of codes, so that list is in the
// contract and in the client stock-flow-web generates from it (ADR 0019). A plain error,
// with no domain code, has the type about:blank: RFC 9457's default, which is what an
// absent type means. Call it after every operation is registered.
func publishErrorCodes(api huma.API) {
	values := []any{"about:blank"}
	meanings := []string{"The error's kind: about:blank for a plain HTTP error, or a domain error code."}
	for _, c := range errorCodes {
		values = append(values, c.code)
		meanings = append(meanings, c.code+": "+c.meaning)
	}
	registry := api.OpenAPI().Components.Schemas
	model := registry.Schema(reflect.TypeFor[huma.ErrorModel](), true, "")
	if model.Ref != "" {
		model = registry.SchemaFromRef(model.Ref)
	}
	model.Properties["type"] = &huma.Schema{
		Type:        huma.TypeString,
		Enum:        values,
		Default:     "about:blank",
		Description: strings.Join(meanings, "\n"),
	}
	// stockProblem's member. ErrorModel allows no property it does not list.
	products := registry.Schema(reflect.TypeFor[[]productStock](), true, "")
	products.Nullable = false
	products.Description = "Only with " + codeInsufficientStock + ": each Product whose stock is too low for the change, with its stock."
	model.Properties["products"] = products
}
