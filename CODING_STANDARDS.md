# Coding standards

`code-review` checks every change against this file (its Standards axis), on top of a fixed baseline of code smells. A rule written here overrides that baseline. Skip anything a tool already enforces; name the tool instead.

## Formatting and linting

- `gofmt` formats; `go vet -tags slow ./...` lints. CI runs both (`ci.yml`), so neither is a review topic.
- Generated code (`internal/store/*.sql.go`, `db.go`, `models.go`, and `openapi.yaml`) is never edited by hand: change the SQL or the Go types, then `go generate ./...`.

## Naming

- Domain concepts use the terms in `stock-flow-context`'s `GLOSSARY.md`, never their `_Avoid_` synonyms.
- Huma `OperationID`s are kebab-case verb-noun (`get-health`, `create-order`): `stock-flow-web`'s generated client names its functions after them, so renaming one is a contract change.
- SQL query names (`-- name: CreateOrder :one`) are PascalCase verb-noun; sqlc turns them into Go methods.

## Structure

- `cmd/api`: the server's `main`, which reads configuration and wires things together. No logic.
- `cmd/openapi`: writes `openapi.yaml`.
- `internal/api`: HTTP only. Huma operations with their input and output types, grouped one file per business area (`health.go`, later `orders.go`…). It depends on what it needs from the database through small interfaces declared here (`api.Store`), so the fast layer can fake them.
- `internal/store`: the database. SQL in `db/queries/*.sql`, the Go generated from it, and `pool.go`. No HTTP types.
- `db/migrations`: goose SQL, numbered sequentially, never edited once merged: fix forward with a new migration.
- Dependencies point inwards: `cmd` → `internal/api` → interfaces satisfied by `internal/store`. `internal/store` imports nothing from `internal/api`.
- A business rule that spans several statements runs in one transaction (`pool.Begin`, then `store.New(tx)`), with constraints in the schema backing the Go checks.
- No root `api/` directory: Vercel would treat it as Serverless Functions (ADR 0008).

## Errors

- Errors reach the client as RFC 9457 Problem Details (ADR 0014): handlers return `huma.Error4xx…`/`huma.Error5xx…`, with `huma.ErrorDetail{Location: "body.quantity", …}` for field errors. Never write an error body by hand.
- Declare every status an operation can return in `huma.Operation.Errors`, so it is in `openapi.yaml`.
- Inside the server, wrap with context, `fmt.Errorf("create order: %w", err)`, and log the cause with `slog` where it becomes a 5xx. A 5xx detail never contains the internal error: no SQL, hosts or credentials.
- Every error the API can return has a code in the published error-code list.

## Comments

- A comment states *why*, or the constraint the code obeys, never *what* the next line does.
- Code that enforces a recorded rule cites it: `<module> R3` or `context ADR 0007`. Code that follows a technical decision of this repo cites it as `ADR 0006`.

## Tests

- Tests assert observable behaviour through the public interface, not internals.
- Assert the actual value, not just "no error" or a status code.
- A test is never weakened, skipped or deleted to get green; challenge it explicitly instead.
- Fast tests drive `internal/api` through `api.New` and `httptest`, with in-memory fakes of its interfaces. Behaviour only Postgres shows (transactions, constraints, locking, the query mode) gets a slow test (`//go:build slow`, `internal/testdb`).
- Test names state the behaviour: `TestHealthReportsUnavailableAsProblemDetailsWhenTheDatabaseFails`.
