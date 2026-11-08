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

type stockMovementProduct struct {
	ID   int64  `json:"id" doc:"Identifies the Product."`
	Name string `json:"name" doc:"The Product's name now."`
	SKU  string `json:"sku" doc:"The Product's SKU now."`
}

type stockMovementUser struct {
	ID   int64  `json:"id" doc:"Identifies the User."`
	Name string `json:"name" doc:"The User's name now."`
}

type stockMovementOrder struct {
	ID     int64 `json:"id" doc:"Identifies the Order."`
	Number int64 `json:"number" doc:"The Order's number."`
}

type stockMovement struct {
	ID         int64                `json:"id" doc:"Identifies the movement. Later movements have higher ids."`
	At         time.Time            `json:"at" doc:"When it was made."`
	Kind       string               `json:"kind" enum:"receive,adjustment,order,cancellation" doc:"receive and adjustment are made on the Stock Movements page; order and cancellation by Orders."`
	Product    stockMovementProduct `json:"product" doc:"The Product whose stock changed."`
	Change     int32                `json:"change" doc:"The change in stock: above 0 adds units, below 0 takes them. Never 0."`
	StockAfter int32                `json:"stockAfter" doc:"The Product's stock just after this movement. Never below 0."`
	User       stockMovementUser    `json:"user" doc:"The User who made it."`
	Note       string               `json:"note" doc:"Why, in up to 200 characters. Empty when there is none; an adjustment always has one."`
	// stock-movements R11: one made by an Order links to it.
	Order *stockMovementOrder `json:"order" doc:"The Order that made it, or null for receive and adjustment."`
}

// TransformSchema marks order nullable, as loginLogEntryBody does for its user: Huma's
// nullable tag panics on a field that refers to another schema.
func (stockMovement) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	order := s.Properties["order"]
	s.Properties["order"] = &huma.Schema{
		Description: order.Description,
		OneOf:       []*huma.Schema{{Ref: order.Ref}, {Type: "null"}},
	}
	return s
}

func stockMovementOf(row store.ListStockMovementsRow) stockMovement {
	m := stockMovement{
		ID: row.ID, At: row.At, Kind: row.Kind,
		Product: stockMovementProduct{ID: row.ProductID, Name: row.ProductName, SKU: row.ProductSku},
		Change:  row.Change, StockAfter: row.StockAfter,
		User: stockMovementUser{ID: row.UserID, Name: row.UserName},
		Note: row.Note,
	}
	if row.OrderID.Valid {
		m.Order = &stockMovementOrder{ID: row.OrderID.Int64, Number: row.OrderNumber.Int64}
	}
	return m
}

type listStockMovementsInput struct {
	ProductID int64  `query:"productId" minimum:"1" doc:"Only the movements of this Product."`
	Kind      string `query:"kind" enum:"receive,adjustment,order,cancellation" doc:"Only the movements of this kind."`
	From      string `query:"from" format:"date" doc:"Only the movements on or after this day of the Shop, in Bangkok time (YYYY-MM-DD)."`
	To        string `query:"to" format:"date" doc:"Only the movements on or before this day of the Shop, in Bangkok time (YYYY-MM-DD)."`
	Before    int64  `query:"before" minimum:"1" doc:"The previous page's nextBefore: only the movements older than that. Leave out for the newest."`
	Limit     int    `query:"limit" minimum:"1" maximum:"100" default:"50" doc:"The most movements on a page."`
}

// Resolve refuses a range that ends before it starts, which no schema can say. Huma has
// checked each date's format by now.
func (in *listStockMovementsInput) Resolve(huma.Context) []error {
	if in.From != "" && in.To != "" && in.To < in.From {
		return []error{&huma.ErrorDetail{Location: "query.to", Message: "must not be before from", Value: in.To}}
	}
	return nil
}

type listStockMovementsOutput struct {
	Body struct {
		Movements  []stockMovement `json:"movements" nullable:"false" doc:"Newest first."`
		NextBefore *int64          `json:"nextBefore" doc:"Pass as before for the next page. Null on the last page."`
	}
}

type receiveStockBody struct {
	ProductID int64 `json:"productId" doc:"The Product received. It must be Active."`
	// stock-movements R4.
	Quantity int32 `json:"quantity" minimum:"1" doc:"How many units arrived: a whole number, at least 1."`
	// stock-movements R2.
	Note string `json:"note,omitempty" maxLength:"200" doc:"Anything worth keeping about the delivery, up to 200 characters."`
}

type receiveStockInput struct {
	Body receiveStockBody
}

type adjustStockBody struct {
	ProductID int64 `json:"productId" doc:"The Product whose stock is wrong. It must be Active."`
	// stock-movements R5: up or down, never 0 (TransformSchema).
	Change int32 `json:"change" doc:"How many units to add (above 0) or take (below 0): a whole number other than 0."`
	// stock-movements R2, R5: required, and not blank.
	Note string `json:"note" maxLength:"200" pattern:"[^\\t\\n\\f\\r ]" patternDescription:"contain a character that is not a space" doc:"Why the stock is wrong, up to 200 characters."`
}

// TransformSchema refuses a change of 0, which Huma's tags cannot say. Being in the schema,
// the refusal is in the contract too.
func (adjustStockBody) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	s.Properties["change"].Not = &huma.Schema{Type: huma.TypeInteger, Enum: []any{0}}
	return s
}

type adjustStockInput struct {
	Body adjustStockBody
}

type stockMovementOutput struct {
	Body stockMovement
}

const stockChangeErrors = "Answers a plain 404 when there is no such Product, 409 `product-archived` when it is Archived, " +
	"and 422 when the stock would grow past 2147483647."

func registerStockMovements(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-stock-movements",
		Method:      http.MethodGet,
		Path:        "/stock-movements",
		Summary:     "List the Stock movements",
		Description: "Every Role sees every Stock movement, newest first, one page at a time. " +
			"No operation changes or removes one. " + sessionErrors,
		Tags:        []string{"Stock movements"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{anyRole(api, deps)},
		Errors:      []int{http.StatusUnauthorized},
	}, func(ctx context.Context, input *listStockMovementsInput) (*listStockMovementsOutput, error) {
		limit := input.Limit
		// One more than the page, to know whether another page follows. to is a whole
		// day: the movements before the next day starts.
		rows, err := deps.Store.ListStockMovements(ctx, store.ListStockMovementsParams{
			ProductID: pgtype.Int8{Int64: input.ProductID, Valid: input.ProductID != 0},
			Kind:      pgtype.Text{String: input.Kind, Valid: input.Kind != ""},
			FromAt:    startOfShopDay(input.From, 0),
			UntilAt:   startOfShopDay(input.To, 1),
			Before:    pgtype.Int8{Int64: input.Before, Valid: input.Before != 0},
			RowLimit:  int32(limit + 1),
		})
		if err != nil {
			return nil, internalError(ctx, "list stock movements", err)
		}
		out := &listStockMovementsOutput{}
		out.Body.Movements = make([]stockMovement, 0, len(rows))
		if len(rows) > limit {
			rows = rows[:limit]
			out.Body.NextBefore = &rows[limit-1].ID
		}
		for _, row := range rows {
			out.Body.Movements = append(out.Body.Movements, stockMovementOf(row))
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "receive-stock",
		Method:      http.MethodPost,
		Path:        "/stock-movements/receive",
		Summary:     "Receive stock",
		Description: "Adds a delivery to a Product's stock, as a receive movement. Every Role may. " +
			stockChangeErrors + " " + sessionErrors,
		Tags:          []string{"Stock movements"},
		Security:      sessionSecurity,
		Middlewares:   huma.Middlewares{anyRole(api, deps)},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict},
	}, func(ctx context.Context, input *receiveStockInput) (*stockMovementOutput, error) {
		received := store.StockChange{ProductID: input.Body.ProductID, Change: input.Body.Quantity}
		return makeMovement(ctx, deps, store.KindReceive, received, input.Body.Note, "body.quantity")
	})

	huma.Register(api, huma.Operation{
		OperationID: "adjust-stock",
		Method:      http.MethodPost,
		Path:        "/stock-movements/adjust",
		Summary:     "Adjust stock",
		Description: "Corrects a Product's stock up or down, as an adjustment movement with a note saying why. " +
			"Answers 409 `insufficient-stock` when it would take the stock below 0, naming the stock there is. " +
			stockChangeErrors + " " + ownerOrAdminErrors,
		Tags:          []string{"Stock movements"},
		Security:      sessionSecurity,
		Middlewares:   huma.Middlewares{ownerOrAdmin(api, deps)},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict},
	}, func(ctx context.Context, input *adjustStockInput) (*stockMovementOutput, error) {
		adjusted := store.StockChange{ProductID: input.Body.ProductID, Change: input.Body.Change}
		return makeMovement(ctx, deps, store.KindAdjustment, adjusted, input.Body.Note, "body.change")
	})
}

// makeMovement makes one movement of kind for the User acting, and answers with it. field
// is where the change came from, for the error when it is too large.
func makeMovement(ctx context.Context, deps Deps, kind string, change store.StockChange, note, field string) (*stockMovementOutput, error) {
	moved, err := deps.Store.MoveStock(ctx, store.MoveStockParams{
		Kind: kind, UserID: sessionOf(ctx).UserID, Now: deps.Now, Note: note, Changes: []store.StockChange{change},
	})
	var short *store.InsufficientStockError
	switch {
	case err == nil:
		return &stockMovementOutput{Body: stockMovementOf(moved[0])}, nil
	case errors.Is(err, store.ErrNotFound):
		return nil, huma.Error404NotFound("There is no such Product.")
	case errors.Is(err, store.ErrProductArchived): // stock-movements R9, products R12
		return nil, domainError(http.StatusConflict, codeProductArchived, "This Product is Archived. Restore it to change its stock.")
	case errors.As(err, &short): // stock-movements R6
		return nil, insufficientStock(short)
	case errors.Is(err, store.ErrStockTooLarge):
		return nil, huma.Error422UnprocessableEntity("The stock would be too large.",
			&huma.ErrorDetail{Location: field, Message: "would take the stock past 2147483647", Value: change.Change})
	}
	return nil, internalError(ctx, "move stock", err)
}

func insufficientStock(err *store.InsufficientStockError) huma.StatusError {
	var products []productStock
	var each []string
	for _, p := range err.Products {
		products = append(products, productStock{ID: p.ProductID, Stock: p.Stock})
		each = append(each, fmt.Sprintf("Only %d in stock.", p.Stock))
	}
	problem := &stockProblem{
		ErrorModel: *domainError(http.StatusConflict, codeInsufficientStock, strings.Join(each, " ")),
		Products:   products,
	}
	return problem
}
