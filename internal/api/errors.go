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
	codeLoginFailed      = "login-failed"
	codeSessionEnded     = "session-ended"
	codeSetupLinkInvalid = "setup-link-invalid"
	codeEmailTaken       = "email-taken"
)

// errorCodes is the published list. Every code above must be in it.
var errorCodes = []struct{ code, meaning string }{
	{codeLoginFailed, "The email and password did not log anyone in. Identical for every cause."},
	{codeSessionEnded, "The request carried a session cookie that matches no live session."},
	{codeSetupLinkInvalid, "The Setup link is used, expired, replaced or was never issued."},
	{codeEmailTaken, "A User has this email already, in any letter case."},
}

func domainError(status int, code, detail string) huma.StatusError {
	return &huma.ErrorModel{Type: code, Title: http.StatusText(status), Status: status, Detail: detail}
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
}
