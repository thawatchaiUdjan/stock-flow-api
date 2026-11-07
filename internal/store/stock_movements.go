package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// The kinds of Stock movement (stock-movements R3).
const (
	KindReceive      = "receive"
	KindAdjustment   = "adjustment"
	KindOrder        = "order"
	KindCancellation = "cancellation"
)

var (
	// ErrProductArchived refuses a movement for an Archived Product (stock-movements R9,
	// products R12). A Cancellation is never refused it (orders R12).
	ErrProductArchived = errors.New("the Product is Archived")
	// ErrStockTooLarge refuses a movement that would take stock past what a stock column
	// holds.
	ErrStockTooLarge = errors.New("the stock would be too large to hold")
)

// ProductStock is a Product's stock.
type ProductStock struct {
	ProductID int64
	Stock     int32
}

// InsufficientStockError refuses movements that would take stock below 0 (stock-movements
// R6, orders R8). It names every Product that would, each with its stock before them.
type InsufficientStockError struct {
	Products []ProductStock
}

func (e *InsufficientStockError) Error() string {
	var each []string
	for _, p := range e.Products {
		each = append(each, fmt.Sprintf("product %d has %d", p.ProductID, p.Stock))
	}
	return "insufficient stock: " + strings.Join(each, ", ")
}

// StockChange is one Product's change in stock.
type StockChange struct {
	ProductID int64
	Change    int32
}

// MoveStockParams is one or more Stock movements of one kind, made at once by one User.
// Now gives their time. It is called once the Products are held, so a movement that waited
// for another is never timed before it, and newest first by id is newest first by time.
type MoveStockParams struct {
	Kind    string
	UserID  int64
	Now     func() time.Time
	Note    string
	Changes []StockChange
}

// MoveStock makes arg's movements and changes the stock they change, in one transaction
// (context ADR 0006). They all apply or none does: it answers ErrNotFound when a Product
// does not exist, ErrProductArchived when one is Archived and arg is not a Cancellation,
// and an *InsufficientStockError when any would take stock below 0. It returns the
// movements, with their Product and User, in the order of arg.Changes.
func (s *Store) MoveStock(ctx context.Context, arg MoveStockParams) (moved []ListStockMovementsRow, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		moved, err = moveStock(ctx, q, arg)
		return err
	})
	if err != nil {
		return nil, err
	}
	return moved, nil
}

// moveStock is the one path every stock change takes, inside a transaction the caller
// holds, so an Order can make its movements in the transaction that creates it.
func moveStock(ctx context.Context, q *Queries, arg MoveStockParams) ([]ListStockMovementsRow, error) {
	ids := make([]int64, 0, len(arg.Changes))
	for _, c := range arg.Changes {
		ids = append(ids, c.ProductID)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	// stock-movements R7: from here until the transaction ends, no other movement for these
	// Products runs, so the stock read here is the stock the movements apply to.
	locked, err := q.LockProducts(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("lock products: %w", err)
	}
	if len(locked) != len(ids) {
		return nil, ErrNotFound
	}
	stock := map[int64]int64{}
	for _, p := range locked {
		// stock-movements R9, products R12; orders R12 gives a Cancellation's stock back
		// even to a Product Archived since.
		if p.Status == statusArchived && arg.Kind != KindCancellation {
			return nil, ErrProductArchived
		}
		stock[p.ID] = int64(p.Stock)
	}

	after := make([]int64, len(arg.Changes))
	for i, c := range arg.Changes {
		stock[c.ProductID] += int64(c.Change)
		after[i] = stock[c.ProductID]
	}
	// stock-movements R6, orders R8: refused whole, naming each Product that falls short.
	var short []ProductStock
	for _, p := range locked {
		if stock[p.ID] < 0 {
			short = append(short, ProductStock{ProductID: p.ID, Stock: p.Stock})
		}
	}
	if short != nil {
		return nil, &InsufficientStockError{Products: short}
	}
	for _, s := range stock {
		if s > math.MaxInt32 {
			return nil, ErrStockTooLarge
		}
	}

	at := arg.Now()
	moved := make([]ListStockMovementsRow, 0, len(arg.Changes))
	for i, c := range arg.Changes {
		id, err := q.InsertStockMovement(ctx, InsertStockMovementParams{
			Kind: arg.Kind, ProductID: c.ProductID, Change: c.Change, StockAfter: int32(after[i]),
			At: at, UserID: arg.UserID, Note: arg.Note,
		})
		if err != nil {
			return nil, fmt.Errorf("insert stock movement: %w", err)
		}
		row, err := q.GetStockMovement(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("get stock movement: %w", err)
		}
		// The same columns as ListStockMovements, which is what the API shows.
		moved = append(moved, ListStockMovementsRow(row))
	}
	for id, s := range stock {
		if err := q.SetProductStock(ctx, SetProductStockParams{ID: id, Stock: int32(s)}); err != nil {
			return nil, fmt.Errorf("set product stock: %w", err)
		}
	}
	return moved, nil
}
