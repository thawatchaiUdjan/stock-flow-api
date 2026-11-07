package storetest

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// checkMovement is what the stock_movements table's constraints refuse.
func (f *Fake) checkMovement(m store.StockMovement) error {
	switch {
	case !slices.Contains([]string{store.KindReceive, store.KindAdjustment, store.KindOrder, store.KindCancellation}, m.Kind):
		return errors.New("violates stock_movements_kind_check")
	case m.Change == 0:
		return errors.New("violates stock_movements_change_check")
	case utf8.RuneCountInString(m.Note) > 200:
		return errors.New("violates stock_movements_note_check")
	case (m.Kind == store.KindReceive || m.Kind == store.KindCancellation) && m.Change < 0,
		m.Kind == store.KindOrder && m.Change > 0:
		return errors.New("violates stock_movements_direction_check")
	case m.Kind == store.KindAdjustment && strings.TrimSpace(m.Note) == "":
		return errors.New("violates stock_movements_adjustment_note_check")
	case (m.Kind == store.KindOrder || m.Kind == store.KindCancellation) != m.OrderID.Valid:
		return errors.New("violates stock_movements_order_check")
	case !slices.ContainsFunc(f.users, func(u store.User) bool { return u.ID == m.UserID }):
		return errors.New("violates stock_movements_user_id_fkey")
	}
	return nil
}

// MoveStock is store.Store's MoveStock, in the order it checks. The mutex stands in for
// the row locks; what they do for real is shown by the slow tests.
func (f *Fake) MoveStock(_ context.Context, arg store.MoveStockParams) ([]store.ListStockMovementsRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	plan, err := f.planMoves(arg, 0)
	if err != nil {
		return nil, err
	}
	return f.applyMoves(plan), nil
}

// movePlan is movements that passed every check, and the stock they leave.
type movePlan struct {
	movements []store.StockMovement
	stock     map[int64]int64
}

// planMoves is every check store.Store's moveStock makes, in its order, changing nothing.
// orderID is the Order the movements link to, or 0.
func (f *Fake) planMoves(arg store.MoveStockParams, orderID int64) (movePlan, error) {
	stock := map[int64]int64{}
	var touched []int64
	for _, c := range arg.Changes {
		p := f.product(c.ProductID)
		if p == nil {
			return movePlan{}, store.ErrNotFound
		}
		if _, seen := stock[c.ProductID]; !seen {
			stock[c.ProductID] = int64(p.Stock)
			touched = append(touched, c.ProductID)
		}
	}
	slices.Sort(touched)
	for _, id := range touched {
		if f.product(id).Status == "archived" && arg.Kind != store.KindCancellation {
			return movePlan{}, store.ErrProductArchived
		}
	}
	at := arg.Now()
	movements := make([]store.StockMovement, 0, len(arg.Changes))
	for _, c := range arg.Changes {
		stock[c.ProductID] += int64(c.Change)
		movements = append(movements, store.StockMovement{
			Kind: arg.Kind, ProductID: c.ProductID, Change: c.Change, StockAfter: int32(stock[c.ProductID]),
			At: at, UserID: arg.UserID, Note: arg.Note, OrderID: pgtype.Int8{Int64: orderID, Valid: orderID != 0},
		})
	}
	var short []store.ProductStock
	for _, id := range touched {
		if stock[id] < 0 {
			short = append(short, store.ProductStock{ProductID: id, Stock: f.product(id).Stock})
		}
	}
	if short != nil {
		return movePlan{}, &store.InsufficientStockError{Products: short}
	}
	for _, id := range touched {
		if stock[id] > math.MaxInt32 {
			return movePlan{}, store.ErrStockTooLarge
		}
	}
	for _, m := range movements {
		if err := f.checkMovement(m); err != nil {
			return movePlan{}, err
		}
	}
	return movePlan{movements: movements, stock: stock}, nil
}

// applyMoves stores what planMoves allowed.
func (f *Fake) applyMoves(plan movePlan) []store.ListStockMovementsRow {
	moved := make([]store.ListStockMovementsRow, 0, len(plan.movements))
	for _, m := range plan.movements {
		m.ID = int64(len(f.movements) + 1)
		f.movements = append(f.movements, m)
		moved = append(moved, f.entry(m))
	}
	for id, s := range plan.stock {
		p := f.product(id)
		p.Stock = int32(s)
		p.StockState = stockState(*p)
	}
	return moved
}

// product is the Product with id, to change in place, or nil when there is none.
func (f *Fake) product(id int64) *store.Product {
	i := slices.IndexFunc(f.products, func(p store.Product) bool { return p.ID == id })
	if i < 0 {
		return nil
	}
	return &f.products[i]
}

// entry is the movement with its Product and User as they are now, as the real query joins
// them.
func (f *Fake) entry(m store.StockMovement) store.ListStockMovementsRow {
	p := f.product(m.ProductID)
	var userName string
	for _, u := range f.users {
		if u.ID == m.UserID {
			userName = u.Name
		}
	}
	row := store.ListStockMovementsRow{
		ID: m.ID, Kind: m.Kind, ProductID: m.ProductID, ProductName: p.Name, ProductSku: p.Sku,
		Change: m.Change, StockAfter: m.StockAfter, At: m.At, UserID: m.UserID, UserName: userName, Note: m.Note,
		OrderID: m.OrderID,
	}
	if o := f.orderByID(m.OrderID.Int64); o != nil {
		row.OrderNumber = pgtype.Int8{Int64: o.Number, Valid: true}
	}
	return row
}

func (f *Fake) ListStockMovements(_ context.Context, arg store.ListStockMovementsParams) ([]store.ListStockMovementsRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	found := []store.ListStockMovementsRow{}
	for i := len(f.movements) - 1; i >= 0 && len(found) < int(arg.RowLimit); i-- {
		m := f.movements[i]
		switch {
		case arg.ProductID.Valid && m.ProductID != arg.ProductID.Int64:
		case arg.Kind.Valid && m.Kind != arg.Kind.String:
		case arg.FromAt.Valid && m.At.Before(arg.FromAt.Time):
		case arg.UntilAt.Valid && !m.At.Before(arg.UntilAt.Time):
		case arg.Before.Valid && m.ID >= arg.Before.Int64:
		default:
			found = append(found, f.entry(m))
		}
	}
	return found, nil
}
