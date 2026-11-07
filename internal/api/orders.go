package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

type orderUser struct {
	ID   int64  `json:"id" doc:"Identifies the User."`
	Name string `json:"name" doc:"The User's name now."`
}

type orderLineProduct struct {
	ID   int64  `json:"id" doc:"Identifies the Product."`
	Name string `json:"name" doc:"The Product's name now."`
	SKU  string `json:"sku" doc:"The Product's SKU now."`
}

type orderLine struct {
	Product  orderLineProduct `json:"product" doc:"The Product sold."`
	Quantity int32            `json:"quantity" doc:"How many units: a whole number, at least 1."`
	Price    int64            `json:"price" doc:"What one unit sold for, in whole satang: the Product's price when the Order was created, which never changes after."`
	Amount   int64            `json:"amount" doc:"The quantity times the price, in whole satang."`
}

type orderFinish struct {
	At time.Time `json:"at" doc:"When."`
	By orderUser `json:"by" doc:"Who."`
}

type order struct {
	ID        int64       `json:"id" doc:"Identifies the Order. A Stock movement made by it carries this id."`
	Number    int64       `json:"number" doc:"The Order's number: counting up from 1 in the order Orders are created, never reused. A refused Order may leave a gap."`
	CreatedAt time.Time   `json:"createdAt" doc:"When it was created."`
	CreatedBy orderUser   `json:"createdBy" doc:"The User who created it."`
	Customer  string      `json:"customer" doc:"The customer's name, up to 120 characters. Empty when there is none."`
	Note      string      `json:"note" doc:"Up to 200 characters. Empty when there is none."`
	Lines     []orderLine `json:"lines" nullable:"false" doc:"In the order they were given. A Product is on one line at most."`
	Total     int64       `json:"total" doc:"The sum of the lines' amounts, in whole satang. There is no tax, discount or delivery charge."`
	Status    string      `json:"status" enum:"pending,completed,cancelled" doc:"Pending until completed or cancelled; completed and cancelled are final."`
	// orders R11, R12, R17: who finished it and when, once it is no longer Pending.
	Completed *orderFinish `json:"completed" doc:"When and by whom it was completed, or null."`
	Cancelled *orderFinish `json:"cancelled" doc:"When and by whom it was cancelled, or null."`
}

// TransformSchema marks completed and cancelled nullable, as stockMovement does for its
// order: Huma's nullable tag panics on a field that refers to another schema.
func (order) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	for _, name := range []string{"completed", "cancelled"} {
		field := s.Properties[name]
		s.Properties[name] = &huma.Schema{
			Description: field.Description,
			OneOf:       []*huma.Schema{{Ref: field.Ref}, {Type: "null"}},
		}
	}
	return s
}

// finishOf is who finished an Order and when, or nil when no one has.
func finishOf(by pgtype.Int8, name pgtype.Text, at pgtype.Timestamptz) *orderFinish {
	if !by.Valid {
		return nil
	}
	return &orderFinish{At: at.Time, By: orderUser{ID: by.Int64, Name: name.String}}
}

func orderOf(d store.OrderDetail) order {
	o := order{
		ID: d.ID, Number: d.Number, CreatedAt: d.CreatedAt, CreatedBy: orderUser{ID: d.CreatedBy, Name: d.CreatedByName},
		Customer: d.Customer, Note: d.Note, Total: d.Total, Status: d.Status,
		Completed: finishOf(d.CompletedBy, d.CompletedByName, d.CompletedAt),
		Cancelled: finishOf(d.CancelledBy, d.CancelledByName, d.CancelledAt),
		Lines:     make([]orderLine, 0, len(d.Lines)),
	}
	for _, l := range d.Lines {
		o.Lines = append(o.Lines, orderLine{
			Product:  orderLineProduct{ID: l.ProductID, Name: l.ProductName, SKU: l.ProductSku},
			Quantity: l.Quantity, Price: l.Price, Amount: int64(l.Quantity) * l.Price, // orders R4
		})
	}
	return o
}

type orderOutput struct {
	Body order
}

type orderSummary struct {
	ID        int64     `json:"id" doc:"Identifies the Order."`
	Number    int64     `json:"number" doc:"The Order's number."`
	CreatedAt time.Time `json:"createdAt" doc:"When it was created."`
	Customer  string    `json:"customer" doc:"The customer's name. Empty when there is none."`
	LineCount int32     `json:"lineCount" doc:"How many lines it has."`
	Total     int64     `json:"total" doc:"The sum of its lines' amounts, in whole satang."`
	Status    string    `json:"status" enum:"pending,completed,cancelled" doc:"Pending, completed or cancelled."`
}

type listOrdersInput struct {
	Search string `query:"search" doc:"Only the Order with this number, and the Orders whose customer contains this text in any letter case. Leave out, or leave blank, for all."`
	Status string `query:"status" enum:"pending,completed,cancelled" doc:"Only the Orders with this status. Leave out for all."`
	Limit  int    `query:"limit" minimum:"1" maximum:"100" default:"50" doc:"The most Orders on a page."`
	Offset int    `query:"offset" minimum:"0" default:"0" doc:"How many matching Orders to skip, to reach a later page."`
}

type listOrdersOutput struct {
	Body struct {
		Orders []orderSummary `json:"orders" nullable:"false" doc:"This page's Orders, newest first."`
		Total  int64          `json:"total" doc:"How many Orders match, whatever the page."`
	}
}

type orderNumberInput struct {
	Number int64 `path:"number" doc:"The Order's number."`
}

type createOrderLine struct {
	ProductID int64 `json:"productId" doc:"The Product. It must be Active, and on no other line."`
	// orders R3.
	Quantity int32 `json:"quantity" minimum:"1" doc:"How many units: a whole number, at least 1."`
}

type createOrderBody struct {
	// orders R1.
	Customer string            `json:"customer,omitempty" maxLength:"120" doc:"The customer's name, up to 120 characters."`
	Note     string            `json:"note,omitempty" maxLength:"200" doc:"Anything worth keeping about the Order, up to 200 characters."`
	Lines    []createOrderLine `json:"lines" minItems:"1" nullable:"false" doc:"One or more lines, each a different Product. Each line's price is the Product's now."`
}

type createOrderInput struct {
	Body createOrderBody
}

// Resolve refuses a Product on two lines (orders R3), which no schema can say.
func (in *createOrderInput) Resolve(huma.Context) []error {
	seen := map[int64]bool{}
	var errs []error
	for i, l := range in.Body.Lines {
		if seen[l.ProductID] {
			errs = append(errs, &huma.ErrorDetail{
				Location: fmt.Sprintf("body.lines[%d].productId", i), Message: "is on another line already", Value: l.ProductID,
			})
		}
		seen[l.ProductID] = true
	}
	return errs
}

const orderNotFoundErrors = "Answers a plain 404 when there is no such Order."

func registerOrders(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-orders",
		Method:      http.MethodGet,
		Path:        "/orders",
		Summary:     "List the Orders",
		Description: "Every Role sees every Order, newest first, one page at a time. " + sessionErrors,
		Tags:        []string{"Orders"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{anyRole(api, deps)},
		Errors:      []int{http.StatusUnauthorized},
	}, func(ctx context.Context, input *listOrdersInput) (*listOrdersOutput, error) {
		search := strings.TrimSpace(input.Search)
		filter := store.CountOrdersParams{
			Status: pgtype.Text{String: input.Status, Valid: input.Status != ""},
			Search: pgtype.Text{String: search, Valid: search != ""},
		}
		rows, err := deps.Store.ListOrders(ctx, store.ListOrdersParams{
			Status: filter.Status, Search: filter.Search, RowOffset: int32(input.Offset), RowLimit: int32(input.Limit),
		})
		if err != nil {
			return nil, internalError(ctx, "list orders", err)
		}
		total, err := deps.Store.CountOrders(ctx, filter)
		if err != nil {
			return nil, internalError(ctx, "count orders", err)
		}
		out := &listOrdersOutput{}
		out.Body.Orders = make([]orderSummary, 0, len(rows))
		for _, row := range rows {
			out.Body.Orders = append(out.Body.Orders, orderSummary{
				ID: row.ID, Number: row.Number, CreatedAt: row.CreatedAt, Customer: row.Customer,
				LineCount: row.LineCount, Total: row.Total, Status: row.Status,
			})
		}
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-order",
		Method:      http.MethodGet,
		Path:        "/orders/{number}",
		Summary:     "Get an Order",
		Description: "Every Role sees every Order, with its lines. " + orderNotFoundErrors + " " + sessionErrors,
		Tags:        []string{"Orders"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{anyRole(api, deps)},
		Errors:      []int{http.StatusUnauthorized, http.StatusNotFound},
	}, func(ctx context.Context, input *orderNumberInput) (*orderOutput, error) {
		got, err := deps.Store.GetOrder(ctx, input.Number)
		if err != nil {
			return nil, orderError(ctx, "get order", err)
		}
		return &orderOutput{Body: orderOf(got)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-order",
		Method:      http.MethodPost,
		Path:        "/orders",
		Summary:     "Create an Order",
		Description: "Every Role may. The Order starts pending, and each line's quantity leaves its Product's stock at once, " +
			"as one order movement per line. It is created whole or not at all: " +
			"409 `insufficient-stock` when any line asks for more than its Product's stock, naming each such Product with its stock; " +
			"409 `product-archived` when a Product is Archived; a plain 404 when there is no such Product; " +
			"and 422 for a Product on two lines or a total past 9223372036854775807. " + sessionErrors,
		Tags:          []string{"Orders"},
		Security:      sessionSecurity,
		Middlewares:   huma.Middlewares{anyRole(api, deps)},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity},
	}, func(ctx context.Context, input *createOrderInput) (*orderOutput, error) {
		lines := make([]store.OrderLineParams, 0, len(input.Body.Lines))
		for _, l := range input.Body.Lines {
			lines = append(lines, store.OrderLineParams{ProductID: l.ProductID, Quantity: l.Quantity})
		}
		created, err := deps.Store.CreateOrder(ctx, store.CreateOrderParams{
			UserID: sessionOf(ctx).UserID, Now: deps.Now, Customer: input.Body.Customer, Note: input.Body.Note, Lines: lines,
		})
		var short *store.InsufficientStockError
		switch {
		case err == nil:
			return &orderOutput{Body: orderOf(created)}, nil
		case errors.Is(err, store.ErrNotFound):
			return nil, huma.Error404NotFound("There is no such Product.")
		case errors.Is(err, store.ErrProductArchived): // orders R6, products R12
			return nil, domainError(http.StatusConflict, codeProductArchived, "A Product on this Order is Archived. Only Active Products can be ordered.")
		case errors.As(err, &short): // orders R8
			return nil, insufficientStock(short)
		case errors.Is(err, store.ErrTotalTooLarge):
			return nil, huma.Error422UnprocessableEntity("The total would be too large.",
				&huma.ErrorDetail{Location: "body.lines", Message: "would make a total past 9223372036854775807"})
		}
		return nil, internalError(ctx, "create order", err)
	})

	huma.Register(api, huma.Operation{
		OperationID: "complete-order",
		Method:      http.MethodPost,
		Path:        "/orders/{number}/complete",
		Summary:     "Complete an Order",
		Description: "Every Role may, once the customer has paid and has the goods. Records who completed it and when; " +
			"no stock changes. Answers 409 `order-not-pending` when it is completed or cancelled already. " +
			orderNotFoundErrors + " " + sessionErrors,
		Tags:        []string{"Orders"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{anyRole(api, deps)},
		Errors:      []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict},
	}, func(ctx context.Context, input *orderNumberInput) (*orderOutput, error) {
		completed, err := deps.Store.CompleteOrder(ctx, store.FinishOrderParams{Number: input.Number, UserID: sessionOf(ctx).UserID, Now: deps.Now})
		if err != nil {
			return nil, orderError(ctx, "complete order", err)
		}
		return &orderOutput{Body: orderOf(completed)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cancel-order",
		Method:      http.MethodPost,
		Path:        "/orders/{number}/cancel",
		Summary:     "Cancel an Order",
		Description: "Records who cancelled it and when, and gives each line's quantity back to its Product's stock " +
			"as one cancellation movement per line, even to a Product Archived since. " +
			"Answers 409 `order-not-pending` when it is completed or cancelled already, " +
			"and a plain 409 when giving the stock back would take it past 2147483647. " +
			orderNotFoundErrors + " " + ownerOrAdminErrors,
		Tags:        []string{"Orders"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{ownerOrAdmin(api, deps)},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict},
	}, func(ctx context.Context, input *orderNumberInput) (*orderOutput, error) {
		cancelled, err := deps.Store.CancelOrder(ctx, store.FinishOrderParams{Number: input.Number, UserID: sessionOf(ctx).UserID, Now: deps.Now})
		if err != nil {
			return nil, orderError(ctx, "cancel order", err)
		}
		return &orderOutput{Body: orderOf(cancelled)}, nil
	})
}

// orderError is the answer of an operation on one Order, when it fails.
func orderError(ctx context.Context, doing string, err error) huma.StatusError {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return huma.Error404NotFound("There is no such Order.")
	case errors.Is(err, store.ErrOrderNotPending): // orders R13
		return domainError(http.StatusConflict, codeOrderNotPending, "This Order is completed or cancelled already.")
	case errors.Is(err, store.ErrStockTooLarge): // a Cancel's stock coming back
		return huma.Error409Conflict("Giving this Order's stock back would take a Product's stock past 2147483647.")
	}
	return internalError(ctx, doing, err)
}
