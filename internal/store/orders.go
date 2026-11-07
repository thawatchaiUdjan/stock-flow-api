package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// The statuses of an Order (orders R10).
const (
	OrderPending   = "pending"
	OrderCompleted = "completed"
	OrderCancelled = "cancelled"
)

var (
	// ErrOrderNotPending refuses to complete or cancel an Order that is Completed or
	// Cancelled already (orders R13).
	ErrOrderNotPending = errors.New("the Order is not Pending")
	// ErrNoLines refuses an Order with no lines (orders R1).
	ErrNoLines = errors.New("an Order needs at least one line")
	// ErrTotalTooLarge refuses an Order whose total would be past what a total column holds.
	ErrTotalTooLarge = errors.New("the Order's total would be too large to hold")
)

// OrderLineParams is one line of an Order being created: a Product and how many.
type OrderLineParams struct {
	ProductID int64
	Quantity  int32
}

// CreateOrderParams is an Order being created by one User. Now gives its time, called once
// its Products are held (as MoveStockParams.Now).
type CreateOrderParams struct {
	UserID   int64
	Now      func() time.Time
	Customer string
	Note     string
	Lines    []OrderLineParams
}

// FinishOrderParams is a User completing or cancelling the Order of Number. Now gives the
// time, called once the Order is held.
type FinishOrderParams struct {
	Number int64
	UserID int64
	Now    func() time.Time
}

// OrderDetail is an Order with its lines, in the order they were given.
type OrderDetail struct {
	ReadOrderRow
	Lines []ListOrderLinesRow
}

// CreateOrder creates a Pending Order and takes each line's quantity from its Product's
// stock, as one Order movement per line, in one transaction (orders R7, context ADR 0007).
// The whole Order is refused, and nothing changes, with ErrNoLines for no lines, ErrNotFound
// when a Product does not exist, ErrProductArchived when one is Archived (orders R6), an
// *InsufficientStockError naming every line that asks for more than its stock (orders R8),
// and ErrTotalTooLarge. A Product on two lines is refused by the schema (orders R3).
func (s *Store) CreateOrder(ctx context.Context, arg CreateOrderParams) (created OrderDetail, err error) {
	if len(arg.Lines) == 0 {
		return OrderDetail{}, ErrNoLines
	}
	err = InTx(ctx, s.pool, func(q *Queries) error {
		ids := make([]int64, 0, len(arg.Lines))
		for _, l := range arg.Lines {
			ids = append(ids, l.ProductID)
		}
		slices.Sort(ids)
		// orders R9: the same locks, in the same order, as moveStock takes below, so the
		// prices read here are the ones the Order is created with.
		locked, err := q.LockProducts(ctx, slices.Compact(ids))
		if err != nil {
			return fmt.Errorf("lock products: %w", err)
		}
		price := map[int64]int64{}
		for _, p := range locked {
			price[p.ID] = p.Price
		}
		changes := make([]StockChange, 0, len(arg.Lines))
		var total int64
		for _, l := range arg.Lines {
			p, ok := price[l.ProductID]
			if !ok {
				return ErrNotFound
			}
			// orders R4. A price is at most 999999999 and a quantity fits in 32 bits, so
			// one amount always fits; only their sum can overflow.
			amount := int64(l.Quantity) * p
			if amount > math.MaxInt64-total {
				return ErrTotalTooLarge
			}
			total += amount
			changes = append(changes, StockChange{ProductID: l.ProductID, Change: -l.Quantity})
		}

		at := arg.Now()
		order, err := q.InsertOrder(ctx, InsertOrderParams{
			Customer: arg.Customer, Note: arg.Note, Total: total, CreatedBy: arg.UserID, CreatedAt: at,
		})
		if err != nil {
			return fmt.Errorf("insert order: %w", err)
		}
		for _, l := range arg.Lines {
			// orders R3: the price is the Product's now, and is never changed after.
			err := q.InsertOrderLine(ctx, InsertOrderLineParams{
				OrderID: order.ID, ProductID: l.ProductID, Quantity: l.Quantity, Price: price[l.ProductID],
			})
			if err != nil {
				return fmt.Errorf("insert order line: %w", err)
			}
		}
		if _, err := moveStock(ctx, q, MoveStockParams{
			Kind: KindOrder, UserID: arg.UserID, Now: func() time.Time { return at }, Changes: changes,
		}, order.ID); err != nil {
			return err
		}
		created, err = orderDetail(ctx, q, order.Number)
		return err
	})
	if err != nil {
		return OrderDetail{}, err
	}
	return created, nil
}

// CompleteOrder completes a Pending Order, recording who and when, and changes no stock
// (orders R11, context ADR 0007). It answers ErrNotFound for no such Order and
// ErrOrderNotPending for one that is Completed or Cancelled (orders R13).
func (s *Store) CompleteOrder(ctx context.Context, arg FinishOrderParams) (OrderDetail, error) {
	return s.finishOrder(ctx, arg, func(q *Queries, id int64) error {
		err := q.SetOrderCompleted(ctx, SetOrderCompletedParams{ID: id, UserID: arg.UserID, At: arg.Now()})
		if err != nil {
			return fmt.Errorf("set order completed: %w", err)
		}
		return nil
	})
}

// CancelOrder cancels a Pending Order, recording who and when, and gives each line's
// quantity back to its Product's stock as one Cancellation movement per line, Archived
// Products included (orders R12). It answers ErrNotFound for no such Order and
// ErrOrderNotPending for one that is Completed or Cancelled (orders R13).
func (s *Store) CancelOrder(ctx context.Context, arg FinishOrderParams) (OrderDetail, error) {
	return s.finishOrder(ctx, arg, func(q *Queries, id int64) error {
		lines, err := q.ListOrderLines(ctx, id)
		if err != nil {
			return fmt.Errorf("list order lines: %w", err)
		}
		changes := make([]StockChange, 0, len(lines))
		for _, l := range lines {
			changes = append(changes, StockChange{ProductID: l.ProductID, Change: l.Quantity})
		}
		moved, err := moveStock(ctx, q, MoveStockParams{Kind: KindCancellation, UserID: arg.UserID, Now: arg.Now, Changes: changes}, id)
		if err != nil {
			return err
		}
		// Cancelled when the stock came back.
		if err := q.SetOrderCancelled(ctx, SetOrderCancelledParams{ID: id, UserID: arg.UserID, At: moved[0].At}); err != nil {
			return fmt.Errorf("set order cancelled: %w", err)
		}
		return nil
	})
}

// finishOrder holds the Pending Order of arg.Number and runs finish on it, in one
// transaction. A Complete and a Cancel at the same moment take the lock one after the other,
// so the second finds the Order finished and changes nothing (orders R13).
func (s *Store) finishOrder(ctx context.Context, arg FinishOrderParams, finish func(q *Queries, id int64) error) (finished OrderDetail, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		order, err := q.LockOrder(ctx, arg.Number)
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock order: %w", err)
		}
		if order.Status != OrderPending {
			return ErrOrderNotPending
		}
		if err := finish(q, order.ID); err != nil {
			return err
		}
		finished, err = orderDetail(ctx, q, arg.Number)
		return err
	})
	if err != nil {
		return OrderDetail{}, err
	}
	return finished, nil
}

// GetOrder is the Order of number with its lines. It answers ErrNotFound for no such Order.
func (s *Store) GetOrder(ctx context.Context, number int64) (OrderDetail, error) {
	return orderDetail(ctx, s.Queries, number)
}

func orderDetail(ctx context.Context, q *Queries, number int64) (OrderDetail, error) {
	order, err := q.ReadOrder(ctx, number)
	if errors.Is(err, ErrNotFound) {
		return OrderDetail{}, ErrNotFound
	}
	if err != nil {
		return OrderDetail{}, fmt.Errorf("read order: %w", err)
	}
	lines, err := q.ListOrderLines(ctx, order.ID)
	if err != nil {
		return OrderDetail{}, fmt.Errorf("list order lines: %w", err)
	}
	return OrderDetail{ReadOrderRow: order, Lines: lines}, nil
}

// orderLink is the Order a movement is made by, or none.
func orderLink(orderID int64) pgtype.Int8 {
	return pgtype.Int8{Int64: orderID, Valid: orderID != 0}
}
