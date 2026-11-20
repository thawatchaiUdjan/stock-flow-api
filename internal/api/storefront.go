package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// The Storefront operations live under /v2/storefront (business ADR 0014, contract 0.3.0).
// Those that need no login need no session cookie, and a StockFlow session never opens a
// Customer operation.

// storefrontProduct is what the Storefront shows of a Product (storefront R3, R4, R18): no
// stock figure but the count a Low Product has left, and no Minimum stock or status.
type storefrontProduct struct {
	ID           int64  `json:"id" doc:"Identifies the Product, for the Cart."`
	Name         string `json:"name" doc:"The Product's name, in any language."`
	SKU          string `json:"sku" doc:"The Product's SKU."`
	Unit         string `json:"unit" doc:"The word the Product is counted in, such as pcs or box."`
	Price        int64  `json:"price" doc:"What one unit sells for, in whole satang (฿1 = 100)."`
	Availability string `json:"availability" enum:"in-stock,low,out-of-stock" doc:"in-stock; low, when the Product is at or below its Minimum stock, with left; or out-of-stock, which cannot be added to the Cart."`
	Left         *int32 `json:"left,omitempty" minimum:"1" doc:"How many units are left, only when availability is low (Only N left). Absent otherwise: no other stock figure is shown."`
}

type listStorefrontProductsInput struct {
	Search string `query:"search" doc:"Only the Products whose name or SKU contains this text, in any letter case. Leave out, or leave blank, for all."`
	Limit  int    `query:"limit" minimum:"1" maximum:"50" default:"25" doc:"The most Products on a page."`
	Offset int    `query:"offset" minimum:"0" default:"0" doc:"How many matching Products to skip, to reach a later page."`
}

type listStorefrontProductsOutput struct {
	Body struct {
		Products []storefrontProduct `json:"products" nullable:"false" doc:"This page's Active Products, sorted by name."`
		Total    int64               `json:"total" minimum:"0" doc:"How many Active Products match, whatever the page. An out-of-range page is empty with the actual total."`
	}
}

func registerStorefront(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-storefront-products",
		Method:      http.MethodGet,
		Path:        "/v2/storefront/products",
		Summary:     "Browse the Storefront catalogue",
		Description: "Public: needs no login. The Active Products only, sorted by name, searched by name or SKU, one bounded page at a time. " +
			"Archived and Deleted Products never appear.",
		Tags:   []string{"Storefront"},
		Errors: []int{http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, func(ctx context.Context, input *listStorefrontProductsInput) (*listStorefrontProductsOutput, error) {
		// storefront R3, products R17.
		search := strings.TrimSpace(input.Search)
		filter := store.CountProductsParams{Status: "active", Search: pgtype.Text{String: search, Valid: search != ""}}
		rows, err := deps.Store.ListProducts(ctx, store.ListProductsParams{
			Status: filter.Status, Search: filter.Search, RowOffset: int32(input.Offset), RowLimit: int32(input.Limit),
		})
		if err != nil {
			return nil, internalError(ctx, "list storefront products", err)
		}
		total, err := deps.Store.CountProducts(ctx, filter)
		if err != nil {
			return nil, internalError(ctx, "count storefront products", err)
		}
		out := &listStorefrontProductsOutput{}
		out.Body.Total = total
		out.Body.Products = make([]storefrontProduct, 0, len(rows))
		for _, row := range rows {
			out.Body.Products = append(out.Body.Products, storefrontProductOf(row))
		}
		return out, nil
	})
}

func storefrontProductOf(row store.Product) storefrontProduct {
	p := storefrontProduct{ID: row.ID, Name: row.Name, SKU: row.Sku, Unit: row.Unit, Price: row.Price, Availability: row.StockState}
	// storefront R4: only a Low Product says how many are left.
	if row.StockState == "low" {
		p.Left = &row.Stock
	}
	return p
}
