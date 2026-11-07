package storetest

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

var skuPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,40}$`)

// AddProduct seeds a Product with any stock, which no operation of the port can set, and
// returns it with its ID and stock state.
func (f *Fake) AddProduct(product store.Product) store.Product {
	f.mu.Lock()
	defer f.mu.Unlock()
	product.ID = int64(len(f.products) + 1)
	product.StockState = stockState(product)
	f.products = append(f.products, product)
	return product
}

// stockState is products R9, which the real database works out in a generated column.
func stockState(p store.Product) string {
	switch {
	case p.Stock == 0:
		return "out-of-stock"
	case p.Stock <= p.MinimumStock:
		return "low"
	}
	return "in-stock"
}

// checkProduct is what the products table's CHECK constraints refuse.
func checkProduct(p store.Product) error {
	switch {
	case strings.TrimSpace(p.Name) == "" || utf8.RuneCountInString(p.Name) > 120:
		return errors.New("violates products_name_check")
	case !skuPattern.MatchString(p.Sku):
		return errors.New("violates products_sku_check")
	case strings.TrimSpace(p.Unit) == "" || utf8.RuneCountInString(p.Unit) > 20:
		return errors.New("violates products_unit_check")
	case p.Price < 0 || p.Price > 999999999:
		return errors.New("violates products_price_check")
	case p.MinimumStock < 0:
		return errors.New("violates products_minimum_stock_check")
	case p.Status != "active" && p.Status != "archived":
		return errors.New("violates products_status_check")
	}
	return nil
}

// skuUsed reports whether a Product other than except has the SKU, in any letter case.
func (f *Fake) skuUsed(sku string, except int64) bool {
	return slices.ContainsFunc(f.products, func(p store.Product) bool {
		return p.ID != except && strings.EqualFold(p.Sku, sku)
	})
}

func (f *Fake) CreateProduct(_ context.Context, arg store.InsertProductParams) (store.Product, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.Product{}, f.Err
	}
	product := store.Product{
		ID: int64(len(f.products) + 1), Name: arg.Name, Sku: arg.Sku, Unit: arg.Unit, Price: arg.Price,
		MinimumStock: arg.MinimumStock, Status: "active",
	}
	// Postgres checks the CHECK constraints before it looks for a conflicting SKU.
	if err := checkProduct(product); err != nil {
		return store.Product{}, err
	}
	if f.skuUsed(arg.Sku, 0) {
		return store.Product{}, store.ErrSKUTaken
	}
	product.StockState = stockState(product)
	f.products = append(f.products, product)
	return product, nil
}

func (f *Fake) GetProduct(_ context.Context, id int64) (store.Product, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.Product{}, f.Err
	}
	for _, product := range f.products {
		if product.ID == id {
			return product, nil
		}
	}
	return store.Product{}, store.ErrNotFound
}

func (f *Fake) UpdateProduct(_ context.Context, arg store.ChangeProductParams) (store.Product, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.Product{}, f.Err
	}
	i := slices.IndexFunc(f.products, func(p store.Product) bool { return p.ID == arg.ID })
	if i < 0 {
		return store.Product{}, store.ErrNotFound
	}
	changed := f.products[i]
	if arg.Name.Valid {
		changed.Name = arg.Name.String
	}
	if arg.Sku.Valid {
		changed.Sku = arg.Sku.String
	}
	if arg.Unit.Valid {
		changed.Unit = arg.Unit.String
	}
	if arg.Price.Valid {
		changed.Price = arg.Price.Int64
	}
	if arg.MinimumStock.Valid {
		changed.MinimumStock = arg.MinimumStock.Int32
	}
	if arg.Status.Valid {
		changed.Status = arg.Status.String
	}
	if err := checkProduct(changed); err != nil {
		return store.Product{}, err
	}
	if f.skuUsed(changed.Sku, changed.ID) {
		return store.Product{}, store.ErrSKUTaken
	}
	changed.StockState = stockState(changed)
	f.products[i] = changed
	return changed, nil
}

// matchingProducts is the Products of the filter, sorted by name as the real query does:
// without regard to letter case, then by bytes, then in the order they were added.
func (f *Fake) matchingProducts(status string, search string, hasSearch bool, states []string) []store.Product {
	var found []store.Product
	for _, p := range f.products {
		switch {
		case p.Status != status:
		case hasSearch && !strings.Contains(strings.ToLower(p.Name), strings.ToLower(search)) &&
			!strings.Contains(strings.ToLower(p.Sku), strings.ToLower(search)):
		case len(states) > 0 && !slices.Contains(states, p.StockState):
		default:
			found = append(found, p)
		}
	}
	slices.SortStableFunc(found, func(a, b store.Product) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return int(a.ID - b.ID)
	})
	return found
}

func (f *Fake) ListProducts(_ context.Context, arg store.ListProductsParams) ([]store.Product, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	found := f.matchingProducts(arg.Status, arg.Search.String, arg.Search.Valid, arg.StockStates)
	found = found[min(int(arg.RowOffset), len(found)):]
	return found[:min(int(arg.RowLimit), len(found))], nil
}

func (f *Fake) CountProducts(_ context.Context, arg store.CountProductsParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return 0, f.Err
	}
	return int64(len(f.matchingProducts(arg.Status, arg.Search.String, arg.Search.Valid, arg.StockStates))), nil
}
