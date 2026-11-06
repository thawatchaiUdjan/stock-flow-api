package api

import (
	"context"
	"net/http"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// Bounds on what a visitor can make the log keep. Anyone can attempt a login, and entries
// are kept indefinitely (users-and-access R39), so an entry's size must not be the
// visitor's to choose.
const (
	maxEmailChars     = 320 // the longest an email address can be
	maxUserAgentChars = 512
)

// Where the visitor's IP address is, in the order they are trusted. Vercel sets these
// itself and discards the visitor's own values (Vercel's "Request headers" documentation).
// X-Vercel-Forwarded-For comes first because Vercel documents X-Forwarded-For as
// overwritable by a proxy in front of it, and the web origin's rewrite may be one
// (ADR 0022). RemoteAddr is the last resort, and what a local run sees.
var clientIPHeaders = []string{"X-Vercel-Forwarded-For", "X-Forwarded-For", "X-Real-Ip"}

type requestInfoKey struct{}

// requestInfo is who sent the request, for the Login log (users-and-access R36).
type requestInfo struct{ ip, userAgent string }

// withRequestInfo keeps the visitor's IP and browser on the request's context, where a
// handler can reach them: Huma gives handlers no *http.Request.
func withRequestInfo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := requestInfo{ip: clientIP(r), userAgent: clip(r.UserAgent(), maxUserAgentChars)}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestInfoKey{}, info)))
	})
}

func clientIP(r *http.Request) string {
	for _, name := range clientIPHeaders {
		first, _, _ := strings.Cut(r.Header.Get(name), ",")
		if ip, err := netip.ParseAddr(strings.TrimSpace(first)); err == nil {
			return ip.String()
		}
	}
	if addr, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
		return addr.Addr().String()
	}
	return ""
}

// clip cuts s to at most n characters, never in the middle of one.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// loginLogEntry is what a request writes to the Login log. It takes no password and the
// entry has no field that could carry one (users-and-access R37). userID is 0 when the
// email matches no User.
func loginLogEntry(ctx context.Context, deps Deps, result, email string, userID int64) store.AddLoginLogEntryParams {
	info, _ := ctx.Value(requestInfoKey{}).(requestInfo)
	return store.AddLoginLogEntryParams{
		At:        deps.Now(),
		Email:     clip(email, maxEmailChars),
		UserID:    pgtype.Int8{Int64: userID, Valid: userID != 0},
		Result:    result,
		Ip:        info.ip,
		UserAgent: info.userAgent,
	}
}

type loginLogUser struct {
	ID   int64  `json:"id" doc:"Identifies the User."`
	Name string `json:"name" doc:"The User's name now."`
}

type loginLogEntryBody struct {
	ID        int64         `json:"id" doc:"Identifies the entry. Later entries have higher ids."`
	At        time.Time     `json:"at" doc:"When it happened."`
	Email     string        `json:"email" doc:"What was entered, as it was typed, even when it matches no User. For a logout, the email of the User who logged out."`
	User      *loginLogUser `json:"user" doc:"The User the email matches, or null when it matches none."`
	Result    string        `json:"result" enum:"login,failed-login,logout" doc:"A successful login, a failed login, or a logout."`
	IP        string        `json:"ip" doc:"The visitor's IP address."`
	UserAgent string        `json:"userAgent" doc:"The browser or device, as it called itself. Empty when it sent nothing."`
}

// TransformSchema marks user nullable. Huma cannot say so for a field that is a reference
// to another schema (its nullable tag panics there), so the field says "this schema or
// null" itself (users-and-access R36: the User, if the email matches one).
func (loginLogEntryBody) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	user := s.Properties["user"]
	s.Properties["user"] = &huma.Schema{
		Description: user.Description,
		OneOf:       []*huma.Schema{{Ref: user.Ref}, {Type: "null"}},
	}
	return s
}

type listLoginLogInput struct {
	UserID int64  `query:"userId" minimum:"1" doc:"Only the entries of this User."`
	Result string `query:"result" enum:"login,failed-login,logout" doc:"Only the entries with this result."`
	Before int64  `query:"before" minimum:"1" doc:"The previous page's nextBefore: only the entries older than that. Leave out for the newest."`
	Limit  int    `query:"limit" minimum:"1" maximum:"100" default:"50" doc:"The most entries on a page."`
}

type listLoginLogOutput struct {
	Body struct {
		Entries    []loginLogEntryBody `json:"entries" nullable:"false" doc:"Newest first."`
		NextBefore *int64              `json:"nextBefore" doc:"Pass as before for the next page. Null on the last page."`
	}
}

func registerLoginLog(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-login-log",
		Method:      http.MethodGet,
		Path:        "/login-log",
		Summary:     "List the Login log",
		Description: "Every login, failed login and logout, newest first, one page at a time. " +
			"Entries cannot be changed or removed, and no operation does. " + ownerOnlyErrors,
		Tags:        []string{"Login log"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{ownerOnly(api, deps)},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
	}, func(ctx context.Context, input *listLoginLogInput) (*listLoginLogOutput, error) {
		// Huma has applied the default (50) and the bounds (1 to 100).
		limit := input.Limit
		// One more than the page, to know whether another page follows.
		rows, err := deps.Store.ListLoginLog(ctx, store.ListLoginLogParams{
			UserID:   pgtype.Int8{Int64: input.UserID, Valid: input.UserID != 0},
			Result:   pgtype.Text{String: input.Result, Valid: input.Result != ""},
			Before:   pgtype.Int8{Int64: input.Before, Valid: input.Before != 0},
			RowLimit: int32(limit + 1),
		})
		if err != nil {
			return nil, internalError(ctx, "list login log", err)
		}
		out := &listLoginLogOutput{}
		out.Body.Entries = make([]loginLogEntryBody, 0, len(rows))
		if len(rows) > limit {
			rows = rows[:limit]
			out.Body.NextBefore = &rows[limit-1].ID
		}
		for _, row := range rows {
			entry := loginLogEntryBody{
				ID: row.ID, At: row.At, Email: row.Email, Result: row.Result, IP: row.Ip, UserAgent: row.UserAgent,
			}
			if row.UserID.Valid {
				entry.User = &loginLogUser{ID: row.UserID.Int64, Name: row.UserName.String}
			}
			out.Body.Entries = append(out.Body.Entries, entry)
		}
		return out, nil
	})
}
