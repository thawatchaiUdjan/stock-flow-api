package storetest

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// bangkok is the Shop's time (dashboard-and-reports R1), which SalesByDay's query applies
// with AT TIME ZONE.
var bangkok = time.FixedZone("Asia/Bangkok", 7*60*60)

// completedIn lists the Orders completed in [from, to).
func (f *Fake) completedIn(from, to time.Time) []store.Order {
	var found []store.Order
	for _, o := range f.orders {
		if o.Status == store.OrderCompleted && !o.CompletedAt.Time.Before(from) && o.CompletedAt.Time.Before(to) {
			found = append(found, o)
		}
	}
	return found
}

func (f *Fake) SalesByDay(_ context.Context, arg store.SalesByDayParams) ([]store.SalesByDayRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	rows := []store.SalesByDayRow{}
	for _, o := range f.completedIn(arg.FromAt, arg.ToAt) {
		day := o.CompletedAt.Time.In(bangkok).Format(time.DateOnly)
		i := slices.IndexFunc(rows, func(r store.SalesByDayRow) bool { return r.Day == day })
		if i < 0 {
			rows = append(rows, store.SalesByDayRow{Day: day})
			i = len(rows) - 1
		}
		rows[i].OrdersCompleted++
		rows[i].Sales += o.Total
	}
	slices.SortFunc(rows, func(a, b store.SalesByDayRow) int { return strings.Compare(a.Day, b.Day) })
	return rows, nil
}

func (f *Fake) TopSellingProducts(_ context.Context, arg store.TopSellingProductsParams) ([]store.TopSellingProductsRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	rows := []store.TopSellingProductsRow{}
	for _, o := range f.completedIn(arg.FromAt, arg.ToAt) {
		for _, l := range f.lines {
			if l.OrderID != o.ID {
				continue
			}
			i := slices.IndexFunc(rows, func(r store.TopSellingProductsRow) bool { return r.ID == l.ProductID })
			if i < 0 {
				p := f.product(l.ProductID)
				rows = append(rows, store.TopSellingProductsRow{ID: p.ID, Name: p.Name, Sku: p.Sku})
				i = len(rows) - 1
			}
			rows[i].QuantitySold += int64(l.Quantity)
			rows[i].Sales += int64(l.Quantity) * l.Price
		}
	}
	slices.SortFunc(rows, func(a, b store.TopSellingProductsRow) int {
		switch {
		case a.Sales != b.Sales:
			return cmpInt(b.Sales, a.Sales)
		case a.QuantitySold != b.QuantitySold:
			return cmpInt(b.QuantitySold, a.QuantitySold)
		}
		return compareNames(a.Name, b.Name, a.ID, b.ID)
	})
	return rows[:min(len(rows), int(arg.RowLimit))], nil
}

func (f *Fake) ListProductsToRestock(_ context.Context, rowLimit int32) ([]store.Product, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	products := []store.Product{}
	for _, p := range f.products {
		if p.Status == "active" && p.StockState != "in-stock" {
			products = append(products, p)
		}
	}
	slices.SortFunc(products, func(a, b store.Product) int {
		if a.Stock != b.Stock {
			return cmpInt(int64(a.Stock), int64(b.Stock))
		}
		return compareNames(a.Name, b.Name, a.ID, b.ID)
	})
	return products[:min(len(products), int(rowLimit))], nil
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// compareNames orders as the queries do: by name without regard to letter case and by
// bytes, then the name itself, then the id.
func compareNames(a, b string, aID, bID int64) int {
	if c := strings.Compare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
		return c
	}
	if c := strings.Compare(a, b); c != 0 {
		return c
	}
	return cmpInt(aID, bID)
}
