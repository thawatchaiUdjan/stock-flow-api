package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

const (
	// dashboard-and-reports R4.
	dashboardDays = 14
	// dashboard-and-reports R5, R6, R7, R8.
	restockLimit    = 10
	newestOrders    = 5
	topProductLimit = 20
	maxReportDays   = 366
)

type salesDay struct {
	Date            string `json:"date" format:"date" doc:"A day of the Shop, in Bangkok time (YYYY-MM-DD)."`
	Sales           int64  `json:"sales" doc:"The totals of the Orders completed that day, in whole satang. 0 when there were none."`
	OrdersCompleted int64  `json:"ordersCompleted" doc:"How many Orders were completed that day."`
}

// salesByDay is one salesDay for each of days days from first, a Shop day's start, taking
// the figures from rows (the days that had Sales). A day with none is 0, so a chart has no
// gaps (dashboard-and-reports R4, R7).
func salesByDay(first time.Time, days int, rows []store.SalesByDayRow) []salesDay {
	byDate := make(map[string]store.SalesByDayRow, len(rows))
	for _, r := range rows {
		byDate[r.Day] = r
	}
	out := make([]salesDay, 0, days)
	for i := range days {
		date := first.AddDate(0, 0, i).Format(time.DateOnly)
		out = append(out, salesDay{Date: date, Sales: byDate[date].Sales, OrdersCompleted: byDate[date].OrdersCompleted})
	}
	return out
}

type restockProduct struct {
	ID           int64  `json:"id" doc:"Identifies the Product."`
	Name         string `json:"name" doc:"The Product's name."`
	SKU          string `json:"sku" doc:"The Product's SKU."`
	Unit         string `json:"unit" doc:"The word the Product is counted in."`
	Stock        int32  `json:"stock" doc:"How many units there are."`
	MinimumStock int32  `json:"minimumStock" doc:"The stock level at or below which the Product needs restocking."`
	StockState   string `json:"stockState" enum:"low,out-of-stock" doc:"Out of stock when the stock is 0, low otherwise."`
}

type dashboardOutput struct {
	Body struct {
		Today           string           `json:"today" format:"date" doc:"The day of the Shop the figures are for, in Bangkok time (YYYY-MM-DD)."`
		Sales           int64            `json:"sales" doc:"Today's Sales: the totals of the Orders completed today, in whole satang."`
		OrdersCompleted int64            `json:"ordersCompleted" doc:"How many Orders were completed today."`
		PendingOrders   int64            `json:"pendingOrders" doc:"How many Orders are Pending now."`
		LowProducts     int64            `json:"lowProducts" doc:"How many Active Products are Low."`
		OutOfStock      int64            `json:"outOfStockProducts" doc:"How many Active Products are Out of stock."`
		SalesByDay      []salesDay       `json:"salesByDay" nullable:"false" doc:"Sales for each of the last 14 days, today included, oldest first. A day with no Sales is 0."`
		Restock         []restockProduct `json:"restock" nullable:"false" doc:"Up to 10 Active Products that are Out of stock or Low: the Out of stock first, then the lowest stock."`
		NewestOrders    []orderSummary   `json:"newestOrders" nullable:"false" doc:"The 5 newest Orders, newest first, as List Orders gives them."`
	}
}

type salesReportInput struct {
	From string `query:"from" format:"date" doc:"The first day of the range, in Bangkok time (YYYY-MM-DD). Leave out for the first day of the month of the last day."`
	To   string `query:"to" format:"date" doc:"The last day of the range, included (YYYY-MM-DD). Leave out for today."`
}

type reportProduct struct {
	ID           int64  `json:"id" doc:"Identifies the Product."`
	Name         string `json:"name" doc:"The Product's name now."`
	SKU          string `json:"sku" doc:"The Product's SKU now."`
	QuantitySold int64  `json:"quantitySold" doc:"How many units were sold in the range."`
	Sales        int64  `json:"sales" doc:"What they sold for, in whole satang: the quantities times the prices of the Orders."`
}

type salesReportOutput struct {
	Body struct {
		From            string          `json:"from" format:"date" doc:"The first day of the range, as it was worked out (YYYY-MM-DD)."`
		To              string          `json:"to" format:"date" doc:"The last day of the range, included."`
		Sales           int64           `json:"sales" doc:"Total Sales in the range: the totals of the Orders completed in it, in whole satang."`
		OrdersCompleted int64           `json:"ordersCompleted" doc:"How many Orders were completed in the range."`
		AverageSales    int64           `json:"averageSales" doc:"Sales per completed Order, in whole satang, rounded half up to the satang. 0 when no Order was completed."`
		SalesByDay      []salesDay      `json:"salesByDay" nullable:"false" doc:"Sales for each day of the range, oldest first. A day with no Sales is 0."`
		TopProducts     []reportProduct `json:"topProducts" nullable:"false" doc:"Up to 20 Products with the highest Sales in the range, the highest first."`
	}
}

// averageSales is total divided by count, rounded half up to the satang; 0 for no Orders
// (the Dashboard and reports spec, Average Sales per Order). Written with the remainder so
// a large total cannot overflow.
func averageSales(total, count int64) int64 {
	if count == 0 {
		return 0
	}
	average := total / count
	if 2*(total%count) >= count {
		average++
	}
	return average
}

const reportRangeErrors = "Answers 422 when `from` is after `to`, or the range is over 366 days."

func registerReports(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "get-dashboard",
		Method:      http.MethodGet,
		Path:        "/dashboard",
		Summary:     "Get the Dashboard",
		Description: "How sales are going and what needs restocking, worked out from the Orders and Products when asked, " +
			"on Bangkok days. Only Completed Orders are Sales. " + ownerOrAdminErrors,
		Tags:        []string{"Dashboard and reports"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{ownerOrAdmin(api, deps)}, // users-and-access R11
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
	}, func(ctx context.Context, _ *struct{}) (*dashboardOutput, error) {
		today := shopDay(deps.Now())
		first := today.AddDate(0, 0, 1-dashboardDays)

		sales, err := deps.Store.SalesByDay(ctx, store.SalesByDayParams{FromAt: first, ToAt: today.AddDate(0, 0, 1)})
		if err != nil {
			return nil, internalError(ctx, "sum sales by day", err)
		}
		pending, err := deps.Store.CountOrders(ctx, store.CountOrdersParams{Status: pgtype.Text{String: store.OrderPending, Valid: true}})
		if err != nil {
			return nil, internalError(ctx, "count pending orders", err)
		}
		counts := map[string]int64{}
		for _, state := range []string{"low", "out-of-stock"} {
			counts[state], err = deps.Store.CountProducts(ctx, store.CountProductsParams{Status: "active", StockStates: []string{state}})
			if err != nil {
				return nil, internalError(ctx, "count products to restock", err)
			}
		}
		restock, err := deps.Store.ListProductsToRestock(ctx, restockLimit)
		if err != nil {
			return nil, internalError(ctx, "list products to restock", err)
		}
		newest, err := deps.Store.ListOrders(ctx, store.ListOrdersParams{RowLimit: newestOrders})
		if err != nil {
			return nil, internalError(ctx, "list newest orders", err)
		}

		out := &dashboardOutput{}
		out.Body.Today = today.Format(time.DateOnly)
		out.Body.SalesByDay = salesByDay(first, dashboardDays, sales)
		todays := out.Body.SalesByDay[dashboardDays-1] // dashboard-and-reports R3
		out.Body.Sales, out.Body.OrdersCompleted = todays.Sales, todays.OrdersCompleted
		out.Body.PendingOrders = pending
		out.Body.LowProducts, out.Body.OutOfStock = counts["low"], counts["out-of-stock"]
		out.Body.Restock = make([]restockProduct, 0, len(restock))
		for _, p := range restock {
			out.Body.Restock = append(out.Body.Restock, restockProduct{
				ID: p.ID, Name: p.Name, SKU: p.Sku, Unit: p.Unit, Stock: p.Stock, MinimumStock: p.MinimumStock, StockState: p.StockState,
			})
		}
		out.Body.NewestOrders = make([]orderSummary, 0, len(newest))
		for _, row := range newest {
			out.Body.NewestOrders = append(out.Body.NewestOrders, orderSummary{
				ID: row.ID, Number: row.Number, CreatedAt: row.CreatedAt, Customer: row.Customer,
				LineCount: row.LineCount, Total: row.Total, Status: row.Status,
			})
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-sales-report",
		Method:      http.MethodGet,
		Path:        "/reports/sales",
		Summary:     "Get the Sales report",
		Description: "Sales over a range of up to 366 days, both ends included, this month so far when `from` and `to` are left out: " +
			"the total, the Orders completed, the average per Order, Sales for each day, and the Products with the highest Sales. " +
			reportRangeErrors + " " + ownerOrAdminErrors,
		Tags:        []string{"Dashboard and reports"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{ownerOrAdmin(api, deps)}, // users-and-access R11
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity},
	}, func(ctx context.Context, input *salesReportInput) (*salesReportOutput, error) {
		// Huma has checked each date's format. dashboard-and-reports R7: this month so far
		// by default.
		last := shopDay(deps.Now())
		if input.To != "" {
			last, _ = time.ParseInLocation(time.DateOnly, input.To, shopTime)
		}
		first := time.Date(last.Year(), last.Month(), 1, 0, 0, 0, 0, shopTime)
		if input.From != "" {
			first, _ = time.ParseInLocation(time.DateOnly, input.From, shopTime)
		}
		if last.Before(first) {
			return nil, huma.Error422UnprocessableEntity("The range ends before it starts.",
				&huma.ErrorDetail{Location: "query.to", Message: "must not be before from", Value: last.Format(time.DateOnly)})
		}
		days := int(last.Sub(first)/(24*time.Hour)) + 1 // a Bangkok day is always 24 hours
		if days > maxReportDays {
			return nil, huma.Error422UnprocessableEntity("The range is too long.",
				&huma.ErrorDetail{Location: "query.from", Message: "range must not be over 366 days", Value: first.Format(time.DateOnly)})
		}
		until := last.AddDate(0, 0, 1)

		sales, err := deps.Store.SalesByDay(ctx, store.SalesByDayParams{FromAt: first, ToAt: until})
		if err != nil {
			return nil, internalError(ctx, "sum sales by day", err)
		}
		top, err := deps.Store.TopSellingProducts(ctx, store.TopSellingProductsParams{FromAt: first, ToAt: until, RowLimit: topProductLimit})
		if err != nil {
			return nil, internalError(ctx, "list top selling products", err)
		}

		out := &salesReportOutput{}
		out.Body.From, out.Body.To = first.Format(time.DateOnly), last.Format(time.DateOnly)
		out.Body.SalesByDay = salesByDay(first, days, sales)
		for _, d := range out.Body.SalesByDay {
			out.Body.Sales += d.Sales
			out.Body.OrdersCompleted += d.OrdersCompleted
		}
		out.Body.AverageSales = averageSales(out.Body.Sales, out.Body.OrdersCompleted)
		out.Body.TopProducts = make([]reportProduct, 0, len(top))
		for _, p := range top {
			out.Body.TopProducts = append(out.Body.TopProducts, reportProduct{
				ID: p.ID, Name: p.Name, SKU: p.Sku, QuantitySold: p.QuantitySold, Sales: p.Sales,
			})
		}
		return out, nil
	})
}
