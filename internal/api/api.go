// Package api is the HTTP surface of stock-flow: routes, request and response types,
// and the OpenAPI contract Huma generates from them.
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// Store is what the handlers need from the database.
type Store interface {
	Ping(ctx context.Context) error
	GetUserByEmail(ctx context.Context, email string) (store.User, error)
	StartSession(ctx context.Context, arg store.StartSessionParams) (bool, error)
	GetSession(ctx context.Context, tokenHash []byte) (store.GetSessionRow, error)
	EndSession(ctx context.Context, arg store.EndSessionParams) error
	AddLoginLogEntry(ctx context.Context, arg store.AddLoginLogEntryParams) error
	ListLoginLog(ctx context.Context, arg store.ListLoginLogParams) ([]store.ListLoginLogRow, error)
	GetSetupLinkUser(ctx context.Context, arg store.GetSetupLinkUserParams) (store.GetSetupLinkUserRow, error)
	SetPassword(ctx context.Context, arg store.SetPasswordParams) (int64, error)
	ListUsers(ctx context.Context) ([]store.ListUsersRow, error)
	CreateUser(ctx context.Context, arg store.CreateUserParams) (store.CreateUserRow, error)
	UpdateUser(ctx context.Context, arg store.ChangeUserParams) (store.ChangeUserRow, error)
	ResetPassword(ctx context.Context, arg store.IssueSetupLinkParams) error
	GetProduct(ctx context.Context, id int64) (store.Product, error)
	ListProducts(ctx context.Context, arg store.ListProductsParams) ([]store.Product, error)
	CountProducts(ctx context.Context, arg store.CountProductsParams) (int64, error)
	MoveStock(ctx context.Context, arg store.MoveStockParams) ([]store.ListStockMovementsRow, error)
	ListStockMovements(ctx context.Context, arg store.ListStockMovementsParams) ([]store.ListStockMovementsRow, error)
	CreateProduct(ctx context.Context, arg store.InsertProductParams) (store.Product, error)
	UpdateProduct(ctx context.Context, arg store.ChangeProductParams) (store.Product, error)
	ListOrders(ctx context.Context, arg store.ListOrdersParams) ([]store.ListOrdersRow, error)
	CountOrders(ctx context.Context, arg store.CountOrdersParams) (int64, error)
	GetOrder(ctx context.Context, number int64) (store.OrderDetail, error)
	CreateOrder(ctx context.Context, arg store.CreateOrderParams) (store.OrderDetail, error)
	CompleteOrder(ctx context.Context, arg store.FinishOrderParams) (store.OrderDetail, error)
	CancelOrder(ctx context.Context, arg store.FinishOrderParams) (store.OrderDetail, error)
	SalesByDay(ctx context.Context, arg store.SalesByDayParams) ([]store.SalesByDayRow, error)
	TopSellingProducts(ctx context.Context, arg store.TopSellingProductsParams) ([]store.TopSellingProductsRow, error)
	ListProductsToRestock(ctx context.Context, rowLimit int32) ([]store.Product, error)
}

// PasswordHasher hashes and verifies passwords. Verify takes "" for a User with no
// password, or no User at all, and answers false in the time a wrong password takes.
type PasswordHasher interface {
	Hash(password string) string
	Verify(encoded, password string) bool
}

// Deps is what the API runs on. The contract needs none of them: no handler runs.
type Deps struct {
	Store          Store
	PasswordHasher PasswordHasher
	Now            func() time.Time
}

// New builds the HTTP handler and the API it serves. The API's OpenAPI document is
// the published contract (openapi.yaml).
func New(deps Deps) (http.Handler, huma.API) {
	router := chi.NewMux()
	router.Use(noStore, withRequestInfo)

	api := humachi.New(router, huma.DefaultConfig("stock-flow API", "0.1.0"))
	registerHealth(api, deps.Store)
	registerSessions(api, deps)
	registerSetupLinks(api, deps)
	registerUsers(api, deps)
	registerLoginLog(api, deps)
	registerProducts(api, deps)
	registerStockMovements(api, deps)
	registerOrders(api, deps)
	registerReports(api, deps)
	publishErrorCodes(api)

	return router, api
}

// noStore forbids caching of every response: the web origin reaches this API through a
// Vercel rewrite, and Vercel caches rewritten responses by the upstream Cache-Control
// (ADR 0016).
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
