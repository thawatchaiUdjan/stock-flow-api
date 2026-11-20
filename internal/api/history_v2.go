package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// The /v2 Stock movements and Login log (business ADR 0014, contract 0.3.0) answer for every
// actor, and name who acted as an actor. The legacy operations keep their shape and cover
// the movements Users made and StockFlow's logins.

type stockMovementV2 struct {
	ID         int64                `json:"id" doc:"Identifies the movement. Later movements have higher ids."`
	At         time.Time            `json:"at" doc:"When it was made."`
	Kind       string               `json:"kind" enum:"receive,adjustment,order,cancellation" doc:"receive and adjustment are made on the Stock Movements page; order and cancellation by Orders."`
	Product    stockMovementProduct `json:"product" doc:"The Product whose stock changed."`
	Change     int32                `json:"change" doc:"The change in stock: above 0 adds units, below 0 takes them. Never 0."`
	StockAfter int32                `json:"stockAfter" doc:"The Product's stock just after this movement. Never below 0."`
	Actor      actor                `json:"actor" doc:"Who made it: a User, a Customer or the system."`
	Note       string               `json:"note" doc:"Why, in up to 200 characters. Empty when there is none; an adjustment always has one."`
	Order      *stockMovementOrder  `json:"order" doc:"The Order that made it, or null for receive and adjustment."`
}

// TransformSchema marks order nullable, as stockMovement does.
func (stockMovementV2) TransformSchema(r huma.Registry, s *huma.Schema) *huma.Schema {
	return stockMovement{}.TransformSchema(r, s)
}

type listStockMovementsV2Output struct {
	Body struct {
		Movements  []stockMovementV2 `json:"movements" nullable:"false" doc:"Newest first."`
		NextBefore *int64            `json:"nextBefore" doc:"Pass as before for the next page. Null on the last page."`
		Total      *int64            `json:"total,omitempty" minimum:"0" doc:"Always present on numbered success: exact count for all current filters before offset/limit, including 0. Omitted in legacy cursor mode."`
	}
}

type loginLogEntryV2 struct {
	ID        int64     `json:"id" doc:"Identifies the entry. Later entries have higher ids."`
	At        time.Time `json:"at" doc:"When it happened."`
	App       string    `json:"app" enum:"stockflow,storefront" doc:"StockFlow, for a User, or the Storefront, for a Customer."`
	Email     string    `json:"email" doc:"What was entered, as it was typed, even when it matches nobody. For a logout, the email of who logged out."`
	Actor     *actor    `json:"actor" doc:"The User or Customer the email matches, or null when it matches none."`
	Result    string    `json:"result" enum:"login,failed-login,logout,blocked" doc:"A successful login, a failed login, a logout, or a login refused because of too many failed logins."`
	IP        string    `json:"ip" doc:"The visitor's IP address."`
	UserAgent string    `json:"userAgent" doc:"The browser or device, as it called itself. Empty when it sent nothing."`
}

// TransformSchema marks actor nullable, as loginLogEntryBody does for its user.
func (loginLogEntryV2) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	field := s.Properties["actor"]
	s.Properties["actor"] = &huma.Schema{
		Description: field.Description,
		OneOf:       []*huma.Schema{{Ref: field.Ref}, {Type: "null"}},
	}
	return s
}

func loginLogEntryV2Of(row store.ListLoginLogRow) loginLogEntryV2 {
	entry := loginLogEntryV2{ID: row.ID, At: row.At, App: row.App, Email: row.Email, Result: row.Result, IP: row.Ip, UserAgent: row.UserAgent}
	switch {
	case row.UserID.Valid:
		a := userActor(row.UserID, row.UserName)
		entry.Actor = &a
	case row.CustomerID.Valid:
		entry.Actor = &actor{Kind: store.ActorCustomer, ID: &row.CustomerID.Int64, Name: row.CustomerName.String}
	}
	return entry
}

// listLoginLogV2Input is listLoginLogInput and an app, written out as listOrdersV2Input is.
type listLoginLogV2Input struct {
	UserID int64         `query:"userId" minimum:"1" doc:"Only the entries of this User."`
	Result string        `query:"result" enum:"login,failed-login,logout,blocked" doc:"Only the entries with this result."`
	App    string        `query:"app" enum:"stockflow,storefront" doc:"Only the entries of this app. Leave out for both."`
	Before int64         `query:"before" minimum:"1" doc:"The previous page's nextBefore: only the entries older than that. Leave out for the newest."`
	Offset historyOffset `query:"offset" minimum:"0" maximum:"2147483647" doc:"Explicitly selects numbered mode, including 0. Omit for legacy cursor mode."`
	Limit  int           `query:"limit" minimum:"1" maximum:"100" default:"50" doc:"The most entries on a page."`
}

func (in *listLoginLogV2Input) Resolve(ctx huma.Context) []error {
	return validateHistoryPagination(ctx)
}

type listLoginLogV2Output struct {
	Body struct {
		Entries    []loginLogEntryV2 `json:"entries" nullable:"false" doc:"Newest first."`
		NextBefore *int64            `json:"nextBefore" doc:"Pass as before for the next page. Null on the last page."`
		Total      *int64            `json:"total,omitempty" minimum:"0" doc:"Always present on numbered success: exact count for all current filters before offset/limit, including 0. Omitted in legacy cursor mode."`
	}
}

func registerHistoryV2(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-stock-movements-v2",
		Method:      http.MethodGet,
		Path:        "/v2/stock-movements",
		Summary:     "List the Stock movements of every actor",
		Description: "Every Role sees every Stock movement, made by a User, a Customer or the system, newest first, one page at a time. " +
			"No operation changes or removes one. " + historyPaginationDescription + " " + sessionErrors,
		Tags:        []string{"Stock movements"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{anyRole(api, deps)},
		Errors:      []int{http.StatusUnauthorized, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, func(ctx context.Context, input *listStockMovementsInput) (*listStockMovementsV2Output, error) {
		page, err := listMovements(ctx, deps, input, false)
		if err != nil {
			return nil, err
		}
		out := &listStockMovementsV2Output{}
		out.Body.NextBefore, out.Body.Total = page.nextBefore, page.total
		out.Body.Movements = make([]stockMovementV2, 0, len(page.rows))
		for _, row := range page.rows {
			m := stockMovementOf(row)
			out.Body.Movements = append(out.Body.Movements, stockMovementV2{
				ID: m.ID, At: m.At, Kind: m.Kind, Product: m.Product, Change: m.Change, StockAfter: m.StockAfter,
				Actor: userOrSystemActor(row.UserID, row.UserName, row.System), Note: m.Note, Order: m.Order,
			})
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-login-log-v2",
		Method:      http.MethodGet,
		Path:        "/v2/login-log",
		Summary:     "List the Login log of both apps",
		Description: "Every login, failed login, blocked login and logout of Users in StockFlow and of Customers on the Storefront, " +
			"newest first, one page at a time. Entries cannot be changed or removed, and no operation does. " +
			historyPaginationDescription + " " + ownerOnlyErrors,
		Tags:        []string{"Login log"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{ownerOnly(api, deps)},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, func(ctx context.Context, input *listLoginLogV2Input) (*listLoginLogV2Output, error) {
		page, err := listLoginLogEntries(ctx, deps, &listLoginLogInput{UserID: input.UserID, Result: input.Result, Before: input.Before, Offset: input.Offset, Limit: input.Limit}, input.App)
		if err != nil {
			return nil, err
		}
		out := &listLoginLogV2Output{}
		out.Body.NextBefore, out.Body.Total = page.nextBefore, page.total
		out.Body.Entries = make([]loginLogEntryV2, 0, len(page.rows))
		for _, row := range page.rows {
			out.Body.Entries = append(out.Body.Entries, loginLogEntryV2Of(row))
		}
		return out, nil
	})
}
