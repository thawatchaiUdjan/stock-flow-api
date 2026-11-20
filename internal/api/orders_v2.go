package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// The /v2 Orders operations (business ADR 0014, contract 0.3.0) answer for every Order,
// In-shop or Online, and name who acted as an actor. The legacy operations keep their shape
// and cover In-shop orders only.

type orderFinishV2 struct {
	At time.Time `json:"at" doc:"When."`
	By actor     `json:"by" doc:"Who: a User, a Customer or, for a cancellation, the system."`
}

type orderV2 struct {
	ID        int64          `json:"id" doc:"Identifies the Order. A Stock movement made by it carries this id."`
	Number    int64          `json:"number" doc:"The Order's number, shared by both channels: counting up from 1, never reused."`
	Channel   string         `json:"channel" enum:"in-shop,online" doc:"In-shop: created by a User in StockFlow. Online: placed by a Customer on the Storefront."`
	CreatedAt time.Time      `json:"createdAt" doc:"When it was created."`
	CreatedBy actor          `json:"createdBy" doc:"Who created it: a User for an In-shop order, the Customer for an Online order."`
	Customer  string         `json:"customer" doc:"The customer's name, up to 120 characters. Empty when there is none."`
	Note      string         `json:"note" doc:"Up to 200 characters. Empty when there is none."`
	Lines     []orderLine    `json:"lines" nullable:"false" doc:"In the order they were given. A Product is on one line at most."`
	Total     int64          `json:"total" doc:"The sum of the lines' amounts, in whole satang."`
	Status    string         `json:"status" enum:"pending,completed,cancelled" doc:"Pending until completed or cancelled; completed and cancelled are final."`
	Completed *orderFinishV2 `json:"completed" doc:"When and by whom it was completed, or null."`
	Cancelled *orderFinishV2 `json:"cancelled" doc:"When and by whom it was cancelled, or null."`
}

// TransformSchema marks completed and cancelled nullable, as order does.
func (orderV2) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	for _, name := range []string{"completed", "cancelled"} {
		field := s.Properties[name]
		s.Properties[name] = &huma.Schema{
			Description: field.Description,
			OneOf:       []*huma.Schema{{Ref: field.Ref}, {Type: "null"}},
		}
	}
	return s
}

func orderV2Of(d store.OrderDetail) orderV2 {
	o := orderV2{
		ID: d.ID, Number: d.Number, Channel: d.Channel, CreatedAt: d.CreatedAt, CreatedBy: orderCreator(d.ReadOrderRow),
		Customer: d.Customer, Note: d.Note, Lines: orderLinesOf(d.Lines), Total: d.Total, Status: d.Status,
	}
	if d.CompletedBy.Valid {
		o.Completed = &orderFinishV2{At: d.CompletedAt.Time, By: userActor(d.CompletedBy, d.CompletedByName)}
	}
	if d.Status == store.OrderCancelled {
		o.Cancelled = &orderFinishV2{At: d.CancelledAt.Time, By: userOrSystemActor(d.CancelledBy, d.CancelledByName, d.CancelledBySystem)}
	}
	return o
}

type orderV2Output struct {
	Body orderV2
}

// orderSummaryV2 is orderSummary and a channel, written out because Huma's schema leaves out
// the fields of an embedded struct.
type orderSummaryV2 struct {
	ID        int64     `json:"id" doc:"Identifies the Order."`
	Number    int64     `json:"number" doc:"The Order's number."`
	Channel   string    `json:"channel" enum:"in-shop,online" doc:"In-shop or Online."`
	CreatedAt time.Time `json:"createdAt" doc:"When it was created."`
	Customer  string    `json:"customer" doc:"The customer's name. Empty when there is none."`
	LineCount int32     `json:"lineCount" doc:"How many lines it has."`
	Total     int64     `json:"total" doc:"The sum of its lines' amounts, in whole satang."`
	Status    string    `json:"status" enum:"pending,completed,cancelled" doc:"Pending, completed or cancelled."`
}

// listOrdersV2Input is listOrdersInput and a channel. Huma applies no defaults to the
// fields of an embedded input struct, so the fields are written out.
type listOrdersV2Input struct {
	Search  string `query:"search" doc:"Only the Order with this number, and the Orders whose customer contains this text in any letter case. Leave out, or leave blank, for all."`
	Status  string `query:"status" enum:"pending,completed,cancelled" doc:"Only the Orders with this status. Leave out for all."`
	Channel string `query:"channel" enum:"in-shop,online" doc:"Only the Orders of this channel. Leave out for both."`
	Limit   int    `query:"limit" minimum:"1" maximum:"100" default:"50" doc:"The most Orders on a page."`
	Offset  int    `query:"offset" minimum:"0" default:"0" doc:"How many matching Orders to skip, to reach a later page."`
}

type listOrdersV2Output struct {
	Body struct {
		Orders []orderSummaryV2 `json:"orders" nullable:"false" doc:"This page's Orders, newest first."`
		Total  int64            `json:"total" doc:"How many Orders match, whatever the page."`
	}
}

func registerOrdersV2(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-orders-v2",
		Method:      http.MethodGet,
		Path:        "/v2/orders",
		Summary:     "List the Orders, In-shop and Online",
		Description: "Every Role sees every Order, newest first, one page at a time, filtered by status, search and channel. " + sessionErrors,
		Tags:        []string{"Orders"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{anyRole(api, deps)},
		Errors:      []int{http.StatusUnauthorized},
	}, func(ctx context.Context, input *listOrdersV2Input) (*listOrdersV2Output, error) {
		rows, total, err := listOrderRows(ctx, deps, &listOrdersInput{Search: input.Search, Status: input.Status, Limit: input.Limit, Offset: input.Offset}, input.Channel)
		if err != nil {
			return nil, err
		}
		out := &listOrdersV2Output{}
		out.Body.Total = total
		out.Body.Orders = make([]orderSummaryV2, 0, len(rows))
		for _, row := range rows {
			out.Body.Orders = append(out.Body.Orders, orderSummaryV2{
				ID: row.ID, Number: row.Number, Channel: row.Channel, CreatedAt: row.CreatedAt, Customer: row.Customer,
				LineCount: row.LineCount, Total: row.Total, Status: row.Status,
			})
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-order-v2",
		Method:      http.MethodGet,
		Path:        "/v2/orders/{number}",
		Summary:     "Get an Order, In-shop or Online",
		Description: "Every Role sees every Order, with its lines and who acted. " + orderNotFoundErrors + " " + sessionErrors,
		Tags:        []string{"Orders"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{anyRole(api, deps)},
		Errors:      []int{http.StatusUnauthorized, http.StatusNotFound},
	}, func(ctx context.Context, input *orderNumberInput) (*orderV2Output, error) {
		got, err := deps.Store.GetOrder(ctx, input.Number)
		if err != nil {
			return nil, orderError(ctx, "get order", err)
		}
		return &orderV2Output{Body: orderV2Of(got)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "complete-order-v2",
		Method:      http.MethodPost,
		Path:        "/v2/orders/{number}/complete",
		Summary:     "Complete an Order, In-shop or Online",
		Description: "Every Role may, once the customer has paid and has the goods. Records who completed it and when; " +
			"no stock changes. Answers 409 `order-not-pending` when it is completed or cancelled already. " +
			orderNotFoundErrors + " " + sessionErrors,
		Tags:        []string{"Orders"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{anyRole(api, deps)},
		Errors:      []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict},
	}, func(ctx context.Context, input *orderNumberInput) (*orderV2Output, error) {
		completed, err := deps.Store.CompleteOrder(ctx, store.FinishOrderParams{Number: input.Number, UserID: sessionOf(ctx).UserID, Now: deps.Now}, activityBy(ctx, deps))
		if err != nil {
			return nil, orderError(ctx, "complete order", err)
		}
		return &orderV2Output{Body: orderV2Of(completed)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cancel-order-v2",
		Method:      http.MethodPost,
		Path:        "/v2/orders/{number}/cancel",
		Summary:     "Cancel an Order, In-shop or Online",
		Description: "Records who cancelled it and when, and gives each line's quantity back to its Product's stock " +
			"as one cancellation movement per line, even to a Product Archived since. " +
			"Answers 409 `order-not-pending` when it is completed or cancelled already, " +
			"and a plain 409 when giving the stock back would take it past 2147483647. " +
			orderNotFoundErrors + " " + ownerOrAdminErrors,
		Tags:        []string{"Orders"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{ownerOrAdmin(api, deps)},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict},
	}, func(ctx context.Context, input *orderNumberInput) (*orderV2Output, error) {
		cancelled, err := deps.Store.CancelOrder(ctx, store.FinishOrderParams{Number: input.Number, UserID: sessionOf(ctx).UserID, Now: deps.Now}, activityBy(ctx, deps))
		if err != nil {
			return nil, orderError(ctx, "cancel order", err)
		}
		return &orderV2Output{Body: orderV2Of(cancelled)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-order-v2",
		Method:      http.MethodDelete,
		Path:        "/v2/orders/{number}",
		Summary:     "Delete an Order, In-shop or Online",
		Description: "Takes a completed or cancelled Order out of the Orders List and out of the Dashboard and Reports, for good: " +
			"nothing restores it. Its stock movements stay, and no stock changes. " +
			"Answers 409 `order-pending` when it is pending. " + orderNotFoundErrors + " It also answers 404 when the Order is deleted already. " +
			ownerOrAdminErrors,
		Tags:          []string{"Orders"},
		Security:      sessionSecurity,
		Middlewares:   huma.Middlewares{ownerOrAdmin(api, deps)},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict},
	}, func(ctx context.Context, input *orderNumberInput) (*struct{}, error) {
		// orders R20
		err := deps.Store.DeleteOrder(ctx, store.DeleteOrderParams{Number: input.Number, UserID: sessionOf(ctx).UserID, At: deps.Now()}, activityBy(ctx, deps))
		if err != nil {
			return nil, orderError(ctx, "delete order", err)
		}
		return nil, nil
	})
}
