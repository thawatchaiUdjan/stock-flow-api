package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

type product struct {
	ID           int64  `json:"id" doc:"Identifies the Product. It never changes."`
	Name         string `json:"name" doc:"The Product's name, in any language."`
	SKU          string `json:"sku" doc:"The Product's SKU, as it was entered. No other Product has it, in any letter case."`
	Unit         string `json:"unit" doc:"The word the Product is counted in, such as pcs or box."`
	Price        int64  `json:"price" doc:"What one unit sells for, in whole satang (฿1 = 100), never a fraction."`
	MinimumStock int32  `json:"minimumStock" doc:"The stock level at or below which the Product needs restocking."`
	Stock        int32  `json:"stock" doc:"How many units there are. It changes only through Stock movements."`
	StockState   string `json:"stockState" enum:"in-stock,low,out-of-stock" doc:"Out of stock when the stock is 0, low when it is above 0 and at or below the Minimum stock, in stock otherwise."`
	Status       string `json:"status" enum:"active,archived" doc:"An Archived Product is no longer sold but still exists, with its stock."`
}

func productOf(row store.Product) product {
	return product{
		ID: row.ID, Name: row.Name, SKU: row.Sku, Unit: row.Unit, Price: row.Price,
		MinimumStock: row.MinimumStock, Stock: row.Stock, StockState: row.StockState, Status: row.Status,
	}
}

type listProductsInput struct {
	Search string   `query:"search" doc:"Only the Products whose name or SKU contains this text, in any letter case. Leave out, or leave blank, for all."`
	Stock  []string `query:"stock,explode" enum:"in-stock,low,out-of-stock" doc:"Only the Products in one of these stock states. Repeat it to name several: stock=low&stock=out-of-stock. Leave out for all."`
	Status string   `query:"status" enum:"active,archived" default:"active" doc:"Only the Products with this status."`
	Limit  int      `query:"limit" minimum:"1" maximum:"100" default:"50" doc:"The most Products on a page."`
	Offset int      `query:"offset" minimum:"0" default:"0" doc:"How many matching Products to skip, to reach a later page."`
}

type listProductsOutput struct {
	Body struct {
		Products []product `json:"products" nullable:"false" doc:"This page's Products, sorted by name."`
		Total    int64     `json:"total" doc:"How many Products match, whatever the page."`
	}
}

type productIDInput struct {
	ID int64 `path:"id" doc:"The Product's id."`
}

type productOutput struct {
	Body product
}

type createProductBody struct {
	// pattern \S: a name of only spaces is no name (products R2). The name is stored as it
	// was given, in any language.
	Name string `json:"name" maxLength:"120" pattern:"\\S" patternDescription:"contain a character that is not a space" doc:"The Product's name, in any language."`
	// products R3: letters, digits and hyphens. The letter case is kept, and ignored when
	// SKUs are compared.
	SKU string `json:"sku" maxLength:"40" pattern:"^[A-Za-z0-9-]+$" patternDescription:"be letters, digits and hyphens only" doc:"Letters, digits and hyphens. No other Product may have it, in any letter case."`
	// products R4.
	Unit string `json:"unit" maxLength:"20" pattern:"\\S" patternDescription:"contain a character that is not a space" doc:"The word the Product is counted in, such as pcs or box."`
	// products R5.
	Price int64 `json:"price" minimum:"0" maximum:"999999999" doc:"What one unit sells for, in whole satang (฿1 = 100), from 0 to 999999999 (฿9,999,999.99)."`
	// products R8. No stock and no status: a new Product starts with a stock of 0 (R7) and
	// Active (R11), and stock is never typed in (R6).
	MinimumStock int32 `json:"minimumStock,omitempty" minimum:"0" default:"0" doc:"The stock level at or below which the Product needs restocking. 0 when left out."`
}

type createProductInput struct {
	Body createProductBody
}

type updateProductBody struct {
	// A field left out stays as it is. There is no stock: nothing sets it here (products
	// R6, context ADR 0006), and a body that names one is refused. Pointers, so that a
	// price or a Minimum stock of 0 is a change and not "left out".
	Name         *string `json:"name,omitempty" maxLength:"120" pattern:"\\S" patternDescription:"contain a character that is not a space" doc:"The Product's name, in any language."`
	SKU          *string `json:"sku,omitempty" maxLength:"40" pattern:"^[A-Za-z0-9-]+$" patternDescription:"be letters, digits and hyphens only" doc:"Letters, digits and hyphens. No other Product may have it, in any letter case."`
	Unit         *string `json:"unit,omitempty" maxLength:"20" pattern:"\\S" patternDescription:"contain a character that is not a space" doc:"The word the Product is counted in."`
	Price        *int64  `json:"price,omitempty" minimum:"0" maximum:"999999999" doc:"What one unit sells for, in whole satang. A new price leaves every Order already created as it was."`
	MinimumStock *int32  `json:"minimumStock,omitempty" minimum:"0" doc:"The stock level at or below which the Product needs restocking."`
	Status       *string `json:"status,omitempty" enum:"active,archived" doc:"Archived takes the Product out of the default list and keeps it, with its stock. Active restores it."`
}

type updateProductInput struct {
	ID   int64 `path:"id" doc:"The Product's id."`
	Body updateProductBody
}

// optionalText is the column of a field the body may leave out.
func optionalText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func registerProducts(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-products",
		Method:      http.MethodGet,
		Path:        "/products",
		Summary:     "List the Products",
		Description: "Every Role sees the same list, prices included. Sorted by name, one page at a time. " +
			"Shows the Active Products unless `status` asks for the Archived ones. " + sessionErrors,
		Tags:        []string{"Products"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{anyRole(api, deps)},
		Errors:      []int{http.StatusUnauthorized},
	}, func(ctx context.Context, input *listProductsInput) (*listProductsOutput, error) {
		// Huma has applied the defaults and the bounds, and refused a stock state that is
		// not one of the three.
		search := strings.TrimSpace(input.Search)
		rows, err := deps.Store.ListProducts(ctx, store.ListProductsParams{
			Status:      input.Status,
			Search:      pgtype.Text{String: search, Valid: search != ""},
			StockStates: input.Stock,
			RowOffset:   int32(input.Offset),
			RowLimit:    int32(input.Limit),
		})
		if err != nil {
			return nil, internalError(ctx, "list products", err)
		}
		total, err := deps.Store.CountProducts(ctx, store.CountProductsParams{
			Status:      input.Status,
			Search:      pgtype.Text{String: search, Valid: search != ""},
			StockStates: input.Stock,
		})
		if err != nil {
			return nil, internalError(ctx, "count products", err)
		}
		out := &listProductsOutput{}
		out.Body.Products = make([]product, 0, len(rows))
		for _, row := range rows {
			out.Body.Products = append(out.Body.Products, productOf(row))
		}
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-product",
		Method:      http.MethodGet,
		Path:        "/products/{id}",
		Summary:     "Get a Product",
		Description: "Answers a plain 404 when there is no such Product. " + sessionErrors,
		Tags:        []string{"Products"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{anyRole(api, deps)},
		Errors:      []int{http.StatusUnauthorized, http.StatusNotFound},
	}, func(ctx context.Context, input *productIDInput) (*productOutput, error) {
		row, err := deps.Store.GetProduct(ctx, input.ID)
		if err != nil {
			return nil, productError(ctx, "get product", err)
		}
		return &productOutput{Body: productOf(row)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-product",
		Method:      http.MethodPost,
		Path:        "/products",
		Summary:     "Create a Product",
		Description: "The Product starts Active with a stock of 0. Answers 409 `sku-taken` when another Product has the SKU, " +
			"in any letter case. " + ownerOrAdminErrors,
		Tags:          []string{"Products"},
		Security:      sessionSecurity,
		Middlewares:   huma.Middlewares{ownerOrAdmin(api, deps)},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict},
	}, func(ctx context.Context, input *createProductInput) (*productOutput, error) {
		row, err := deps.Store.CreateProduct(ctx, store.InsertProductParams{
			Name: input.Body.Name, Sku: input.Body.SKU, Unit: input.Body.Unit,
			Price: input.Body.Price, MinimumStock: input.Body.MinimumStock,
		})
		if err != nil {
			return nil, productError(ctx, "create product", err)
		}
		return &productOutput{Body: productOf(row)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-product",
		Method:      http.MethodPatch,
		Path:        "/products/{id}",
		Summary:     "Change a Product",
		Description: "Changes the fields the body names and leaves the rest. Never the stock. " +
			"Answers a plain 404 when there is no such Product, and 409 `sku-taken` when another Product has the new SKU, " +
			"in any letter case. " + ownerOrAdminErrors,
		Tags:        []string{"Products"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{ownerOrAdmin(api, deps)},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict},
	}, func(ctx context.Context, input *updateProductInput) (*productOutput, error) {
		body := input.Body
		change := store.ChangeProductParams{
			ID: input.ID, Name: optionalText(body.Name), Sku: optionalText(body.SKU), Unit: optionalText(body.Unit), Status: optionalText(body.Status),
		}
		if body.Price != nil {
			change.Price = pgtype.Int8{Int64: *body.Price, Valid: true}
		}
		if body.MinimumStock != nil {
			change.MinimumStock = pgtype.Int4{Int32: *body.MinimumStock, Valid: true}
		}
		row, err := deps.Store.UpdateProduct(ctx, change)
		if err != nil {
			return nil, productError(ctx, "update product", err)
		}
		return &productOutput{Body: productOf(row)}, nil
	})
}

// productError is the answer of an operation on a Product, when it fails.
func productError(ctx context.Context, doing string, err error) huma.StatusError {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return huma.Error404NotFound("There is no such Product.")
	case errors.Is(err, store.ErrSKUTaken): // products R3
		return domainError(http.StatusConflict, codeSKUTaken, "Another Product has this SKU.")
	}
	return internalError(ctx, doing, err)
}
