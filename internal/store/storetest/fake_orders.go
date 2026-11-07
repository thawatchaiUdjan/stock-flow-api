package storetest

import (
	"context"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// CreateOrder is store.Store's CreateOrder, in the order it checks, then what the orders
// and order_lines tables' constraints refuse, then moveStock's checks. Nothing is kept
// until all pass, as in its transaction; the Order's number is used up all the same, as
// the sequence's is.
func (f *Fake) CreateOrder(_ context.Context, arg store.CreateOrderParams) (store.OrderDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.OrderDetail{}, f.Err
	}
	if len(arg.Lines) == 0 {
		return store.OrderDetail{}, store.ErrNoLines
	}
	var total int64
	changes := make([]store.StockChange, 0, len(arg.Lines))
	for _, l := range arg.Lines {
		p := f.product(l.ProductID)
		if p == nil {
			return store.OrderDetail{}, store.ErrNotFound
		}
		amount := int64(l.Quantity) * p.Price
		if amount > math.MaxInt64-total {
			return store.OrderDetail{}, store.ErrTotalTooLarge
		}
		total += amount
		changes = append(changes, store.StockChange{ProductID: l.ProductID, Change: -l.Quantity})
	}

	at := arg.Now()
	f.numbered++
	order := store.Order{
		ID: int64(len(f.orders) + 1), Number: f.numbered, Customer: arg.Customer, Note: arg.Note,
		Total: total, Status: store.OrderPending, CreatedBy: arg.UserID, CreatedAt: at,
	}
	switch {
	case utf8.RuneCountInString(order.Customer) > 120:
		return store.OrderDetail{}, errors.New("violates orders_customer_check")
	case utf8.RuneCountInString(order.Note) > 200:
		return store.OrderDetail{}, errors.New("violates orders_note_check")
	case total < 0:
		return store.OrderDetail{}, errors.New("violates orders_total_check")
	case f.userRef(arg.UserID) == nil:
		return store.OrderDetail{}, errors.New("violates orders_created_by_fkey")
	}
	var seen []int64
	for _, l := range arg.Lines {
		switch {
		case l.Quantity < 1:
			return store.OrderDetail{}, errors.New("violates order_lines_quantity_check")
		case slices.Contains(seen, l.ProductID):
			return store.OrderDetail{}, errors.New("violates order_lines_order_product_key")
		}
		seen = append(seen, l.ProductID)
	}
	plan, err := f.planMoves(store.MoveStockParams{
		Kind: store.KindOrder, UserID: arg.UserID, Now: func() time.Time { return at }, Changes: changes,
	}, order.ID)
	if err != nil {
		return store.OrderDetail{}, err
	}

	f.orders = append(f.orders, order)
	for _, l := range arg.Lines {
		f.lines = append(f.lines, store.OrderLine{
			ID: int64(len(f.lines) + 1), OrderID: order.ID, ProductID: l.ProductID, Quantity: l.Quantity,
			Price: f.product(l.ProductID).Price,
		})
	}
	f.applyMoves(plan)
	return f.detail(order), nil
}

func (f *Fake) CompleteOrder(_ context.Context, arg store.FinishOrderParams) (store.OrderDetail, error) {
	return f.finishOrder(arg, func(o *store.Order) error {
		o.Status = store.OrderCompleted
		o.CompletedBy = pgtype.Int8{Int64: arg.UserID, Valid: true}
		o.CompletedAt = pgtype.Timestamptz{Time: arg.Now(), Valid: true}
		return nil
	})
}

func (f *Fake) CancelOrder(_ context.Context, arg store.FinishOrderParams) (store.OrderDetail, error) {
	return f.finishOrder(arg, func(o *store.Order) error {
		var changes []store.StockChange
		for _, l := range f.lines {
			if l.OrderID == o.ID {
				changes = append(changes, store.StockChange{ProductID: l.ProductID, Change: l.Quantity})
			}
		}
		plan, err := f.planMoves(store.MoveStockParams{Kind: store.KindCancellation, UserID: arg.UserID, Now: arg.Now, Changes: changes}, o.ID)
		if err != nil {
			return err
		}
		moved := f.applyMoves(plan)
		o.Status = store.OrderCancelled
		o.CancelledBy = pgtype.Int8{Int64: arg.UserID, Valid: true}
		o.CancelledAt = pgtype.Timestamptz{Time: moved[0].At, Valid: true}
		return nil
	})
}

// finishOrder is store.Store's finishOrder. finish changes the Order in place.
func (f *Fake) finishOrder(arg store.FinishOrderParams, finish func(*store.Order) error) (store.OrderDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.OrderDetail{}, f.Err
	}
	o := f.orderByNumber(arg.Number)
	if o == nil {
		return store.OrderDetail{}, store.ErrNotFound
	}
	if o.Status != store.OrderPending {
		return store.OrderDetail{}, store.ErrOrderNotPending
	}
	if f.userRef(arg.UserID) == nil {
		return store.OrderDetail{}, errors.New("violates an orders user fkey")
	}
	if err := finish(o); err != nil {
		return store.OrderDetail{}, err
	}
	return f.detail(*o), nil
}

func (f *Fake) GetOrder(_ context.Context, number int64) (store.OrderDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.OrderDetail{}, f.Err
	}
	o := f.orderByNumber(number)
	if o == nil {
		return store.OrderDetail{}, store.ErrNotFound
	}
	return f.detail(*o), nil
}

func (f *Fake) ListOrders(_ context.Context, arg store.ListOrdersParams) ([]store.ListOrdersRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	found := f.findOrders(arg.Status, arg.Search)
	rows := []store.ListOrdersRow{}
	for i := int(arg.RowOffset); i < len(found) && len(rows) < int(arg.RowLimit); i++ {
		o := found[i]
		var count int32
		for _, l := range f.lines {
			if l.OrderID == o.ID {
				count++
			}
		}
		rows = append(rows, store.ListOrdersRow{
			ID: o.ID, Number: o.Number, CreatedAt: o.CreatedAt, Customer: o.Customer, LineCount: count, Total: o.Total, Status: o.Status,
		})
	}
	return rows, nil
}

func (f *Fake) CountOrders(_ context.Context, arg store.CountOrdersParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return 0, f.Err
	}
	return int64(len(f.findOrders(arg.Status, arg.Search))), nil
}

// findOrders is the WHERE and ORDER BY of ListOrders.
func (f *Fake) findOrders(status, search pgtype.Text) []store.Order {
	var found []store.Order
	for _, o := range slices.Backward(f.orders) {
		switch {
		case status.Valid && o.Status != status.String:
		case search.Valid && strconv.FormatInt(o.Number, 10) != search.String &&
			!strings.Contains(strings.ToLower(o.Customer), strings.ToLower(search.String)):
		default:
			found = append(found, o)
		}
	}
	return found
}

// detail is the Order with its lines, and its Users' and Products' names as they are now.
func (f *Fake) detail(o store.Order) store.OrderDetail {
	name := func(id pgtype.Int8) pgtype.Text {
		if u := f.userRef(id.Int64); id.Valid && u != nil {
			return pgtype.Text{String: u.Name, Valid: true}
		}
		return pgtype.Text{}
	}
	d := store.OrderDetail{ReadOrderRow: store.ReadOrderRow{
		ID: o.ID, Number: o.Number, Customer: o.Customer, Note: o.Note, Total: o.Total, Status: o.Status,
		CreatedBy: o.CreatedBy, CreatedByName: name(pgtype.Int8{Int64: o.CreatedBy, Valid: true}).String, CreatedAt: o.CreatedAt,
		CompletedBy: o.CompletedBy, CompletedByName: name(o.CompletedBy), CompletedAt: o.CompletedAt,
		CancelledBy: o.CancelledBy, CancelledByName: name(o.CancelledBy), CancelledAt: o.CancelledAt,
	}}
	for _, l := range f.lines {
		if l.OrderID == o.ID {
			p := f.product(l.ProductID)
			d.Lines = append(d.Lines, store.ListOrderLinesRow{
				ProductID: l.ProductID, ProductName: p.Name, ProductSku: p.Sku, Quantity: l.Quantity, Price: l.Price,
			})
		}
	}
	return d
}

func (f *Fake) orderByNumber(number int64) *store.Order {
	i := slices.IndexFunc(f.orders, func(o store.Order) bool { return o.Number == number })
	if i < 0 {
		return nil
	}
	return &f.orders[i]
}

func (f *Fake) orderByID(id int64) *store.Order {
	i := slices.IndexFunc(f.orders, func(o store.Order) bool { return o.ID == id })
	if i < 0 {
		return nil
	}
	return &f.orders[i]
}
