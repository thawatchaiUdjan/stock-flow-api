# Coding standards

`code-review` checks every change against this file (its Standards axis), on top of a fixed baseline of code smells. A rule written here overrides that baseline. Skip anything a tool already enforces; name the tool instead.

## Formatting and linting

- `gofmt` formats; `go vet -tags slow ./...` lints. CI runs both (`ci.yml`), so neither is a review topic.
- Generated code (`internal/store/*.sql.go`, `db.go`, `models.go`, and `openapi.yaml`) is never edited by hand: change the SQL or the Go types, then `go generate ./...`.

## Naming

- Domain concepts use the terms in this repo's `GLOSSARY.md`, never their `_Avoid_` synonyms.
- Huma `OperationID`s are kebab-case verb-noun (`get-health`, `create-order`): `stock-flow-web`'s generated client names its functions after them, so renaming one is a contract change.
- SQL query names (`-- name: CreateOrder :one`) are PascalCase verb-noun; sqlc turns them into Go methods.

## Structure

- `cmd/api`: the server's `main`, which reads configuration and wires things together. No logic.
- `cmd/openapi`: writes `openapi.yaml`.
- `cmd/install`: the developer command that issues the Owner's Setup link. Wiring, flags and printing; no logic.
- `internal/api`: HTTP only. Huma operations with their input and output types, grouped one file per business area (`health.go`, later `orders.go`…). It depends on what it needs from the database through small interfaces declared here (`api.Store`), so the fast layer can fake them.
- `internal/auth`: what logging in rests on outside HTTP: password hashing, tokens, issuing the Owner's Setup link.
- `internal/store`: the database. SQL in `db/queries/*.sql`, the Go generated from it, `pool.go`, and `store.go`: `store.Store`, what the API runs on, with the rules that take several statements. No HTTP types.
- `internal/store/storetest`: the contract suite of the store ports and the fake that passes it. Imported by tests only.
- `internal/testdb`: real Postgres for slow tests. Imported only by `//go:build slow` files.
- `db/migrations`: goose SQL, numbered sequentially, never edited once merged: fix forward with a new migration. A new table enables Row Level Security, with no policy, in the migration that creates it (ADR 0021).
- Dependencies point inwards: `cmd` → `internal/api` → `internal/auth` → `internal/store`, with the database reached through interfaces `internal/store` satisfies. `internal/store` imports nothing from `internal/api` or `internal/auth`.
- A business rule that spans several statements runs in one transaction (`store.InTx`), with constraints in the schema backing the Go checks.
- No root `api/` directory: Vercel would treat it as Serverless Functions (ADR 0008).

## Errors

- Errors reach the client as RFC 9457 Problem Details (ADR 0014): handlers return `huma.Error4xx…`/`huma.Error5xx…`, with `huma.ErrorDetail{Location: "body.quantity", …}` for field errors. Never write an error body by hand.
- Declare every status an operation can return in `huma.Operation.Errors`, so it is in `openapi.yaml`.
- Inside the server, wrap with context, `fmt.Errorf("create order: %w", err)`, and log the cause with `slog` where it becomes a 5xx. A 5xx detail never contains the internal error: no SQL, hosts or credentials.
- A domain error is returned with `domainError`, its code in Problem Details' `type`, and that code is listed in `errorCodes` (`internal/api/errors.go`), which publishes it in `openapi.yaml` (ADR 0019). Field errors keep `errors[].location`.

## Comments

- A comment states *why*, or the constraint the code obeys, never *what* the next line does.
- Code that enforces a recorded rule cites it: `<module> R3` or `business ADR 0007` (historical `context ADR 0007` resolves to the same imported decision). Code that follows a technical decision of this repo cites it as `ADR 0006`.

## Tests

- Tests assert observable behaviour through the public interface, not internals.
- Assert the actual value, not just "no error" or a status code.
- A test is never weakened, skipped or deleted to get green; challenge it explicitly instead.
- Fast tests drive `internal/api` through `api.New` and `httptest`, with in-memory fakes of its interfaces. Behaviour only Postgres shows (transactions, constraints, locking, the query mode) gets a slow test (`//go:build slow`, `internal/testdb`).
- Test names state the behaviour: `TestHealthReportsUnavailableAsProblemDetailsWhenTheDatabaseFails`.
