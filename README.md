# Wallet Service

A single-currency wallet service built as a coding challenge. Supports creating wallets, depositing and withdrawing funds, checking balances, and listing transactions with cursor-based pagination.

## Architecture

The service follows a hexagonal (ports & adapters) architecture, keeping the domain logic isolated from infrastructure concerns.

```
┌──────────────────────────────────────────────────────────┐
│                     HTTP Delivery                         │
│            router · handlers · middleware                 │
└────────────────────────┬─────────────────────────────────┘
                         │ calls
┌────────────────────────▼─────────────────────────────────┐
│                  Application Layer                        │
│   CreateWallet · Deposit · Withdraw · CheckBalance        │
│                  ListTransactions                         │
└────────────────────────┬─────────────────────────────────┘
                         │ uses ports (interfaces)
┌────────────────────────▼─────────────────────────────────┐
│                    Domain Layer                           │
│           Wallet · Transaction · Money                    │
│         WalletRepository · Transactor (ports)            │
└────────────────────────┬─────────────────────────────────┘
                         │ implemented by
┌────────────────────────▼─────────────────────────────────┐
│                 Infrastructure Layer                      │
│          PostgreSQL repository · Transactor               │
│               pgxpool · raw SQL · migrations              │
└──────────────────────────────────────────────────────────┘
```

### Request flow

```
Client
  │
  ▼
HTTP Handler        (parse & validate request, map errors to HTTP)
  │
  ▼
Use Case            (orchestrate domain logic)
  │
  ├──▶ Domain       (Wallet.Credit / Wallet.Debit — pure business rules)
  │
  └──▶ Repository   (persist via SELECT FOR UPDATE + DB transaction)
  │
  ▼
HTTP Handler        (serialize response)
  │
  ▼
Client
```

### Key design decisions

| Concern | Approach |
|---|---|
| Money precision | `shopspring/decimal` — never `float64` |
| Concurrency | `SELECT FOR UPDATE` inside a DB transaction on every mutation |
| Double-spend | Row-level lock prevents concurrent withdrawals on the same wallet |
| Idempotency | Optional `reference` field — unique per wallet when non-empty |
| Pagination | Cursor-based (`created_at`, `id`) — stable tiebreaker prevents skipped rows under concurrent inserts |
| Logging | Structured JSON via `slog` — request ID, wallet ID, amount, and outcome on every mutation |
| Shutdown | Graceful — drains in-flight requests before closing (10s timeout) |
| Authentication | Out of scope — expected to be handled upstream (e.g. API gateway or auth middleware) before requests reach this service |
| Rate limiting | Out of scope — expected to be enforced at the API gateway layer |

---

## Idempotency & safe retries

Deposit and withdraw requests accept an optional `reference` field. When a non-empty reference is provided, the service enforces uniqueness per wallet: a second request with the same reference on the same wallet returns `409 TRANSACTION_EXISTS` rather than applying the operation again.

This makes it safe to retry failed or timed-out requests:

```
Client                          Service
  │                                │
  ├─── POST /deposit ref=txn-1 ───▶│
  │         (timeout / crash)      │ ← DB commit succeeded
  │                                │
  ├─── POST /deposit ref=txn-1 ───▶│ ← duplicate detected
  │◀── 409 TRANSACTION_EXISTS ─────│
  │                                │
```

The `409` response signals "already done" — the money moved exactly once. The client can treat this as a success and re-fetch the wallet balance or transaction list if it needs the original response body.

> Note: the same `reference` can be reused across different wallets, and across different operation types on the same wallet (e.g. a deposit and a withdrawal can share the same reference). Uniqueness is scoped to `(wallet_id, type, reference)`.

---

## Authentication

Authentication is intentionally out of scope for this service. In a production deployment it would be handled upstream — by an API gateway, a reverse proxy, or a shared auth middleware — before requests arrive here. The `owner_id` field on each wallet is designed to carry the verified caller identity once auth is wired in.

---

## Observability

- **Structured logging** — all output is JSON via `slog`. Every mutating request logs `wallet_id`, `amount`, `reference`, `transaction_id`, and outcome at `INFO` level. Business errors (insufficient funds, duplicate reference, wallet not found) log at `WARN`; infrastructure failures log at `ERROR`.
- **Request IDs** — the `X-Request-ID` header is propagated through the request lifecycle and included in all log lines. If the client does not send one, the service generates a UUID automatically.
- **Health check** — `GET /health` pings the database and returns `200 {"status":"ok"}` or `503 {"status":"unavailable"}`. Suitable for Kubernetes liveness and readiness probes.

---

## Getting started

### Prerequisites

- Docker + Docker Compose
- Go 1.22+

### Run everything in Docker

```bash
make run
```

### Run DB in Docker, app locally (faster dev loop)

```bash
make dev
```

### Environment variables

| Variable | Description | Default |
|---|---|---|
| `DATABASE_URL` | Postgres connection string | required |
| `PORT` | HTTP port | `8080` |

Connection pool settings (`MaxConns=25`, `MinConns=5`, `MaxConnLifetime=30m`, `MaxConnIdleTime=5m`) are hardcoded in `cmd/server/main.go`.

Copy `.env.example` to `.env` and adjust as needed.

---

## API

Base URL: `http://localhost:8080`

All request and response bodies are JSON. Money values are strings to preserve decimal precision.

---

### Health check

```
GET /health
```

**Response** `200 OK`
```json
{ "status": "ok" }
```

Returns `503 Service Unavailable` with `{"status":"unavailable"}` if the database is unreachable.

---

### Create wallet

```
POST /wallets
```

**Request**
```json
{ "owner_id": "user-123" }
```

**Response** `201 Created`
```json
{
  "id": "a1b2c3d4-...",
  "owner_id": "user-123",
  "balance": "0",
  "created_at": "2024-01-15T10:00:00Z",
  "updated_at": "2024-01-15T10:00:00Z"
}
```

**Errors**

| Status | Code | Reason |
|---|---|---|
| 400 | `INVALID_OWNER` | Empty owner ID |
| 409 | `WALLET_EXISTS` | Owner already has a wallet |

---

### Deposit

```
POST /wallets/{id}/deposit
```

**Request**
```json
{
  "amount": "100.00",
  "reference": "txn-abc-123"
}
```

`reference` is optional but strongly recommended — see [Idempotency & safe retries](#idempotency--safe-retries).

**Response** `201 Created`
```json
{
  "id": "txn-uuid-...",
  "wallet_id": "a1b2c3d4-...",
  "type": "deposit",
  "amount": "100.00",
  "balance_after": "100.00",
  "reference": "txn-abc-123",
  "created_at": "2024-01-15T10:01:00Z"
}
```

**Errors**

| Status | Code | Reason |
|---|---|---|
| 400 | `INVALID_WALLET_ID` | Malformed UUID |
| 400 | `INVALID_AMOUNT` | Non-positive or unparseable amount |
| 404 | `WALLET_NOT_FOUND` | Wallet does not exist |
| 409 | `TRANSACTION_EXISTS` | Reference already used on this wallet |

---

### Withdraw

```
POST /wallets/{id}/withdraw
```

**Request**
```json
{
  "amount": "40.00",
  "reference": "txn-xyz-456"
}
```

`reference` is optional but strongly recommended — see [Idempotency & safe retries](#idempotency--safe-retries).

**Response** `201 Created`
```json
{
  "id": "txn-uuid-...",
  "wallet_id": "a1b2c3d4-...",
  "type": "withdraw",
  "amount": "40.00",
  "balance_after": "60.00",
  "reference": "txn-xyz-456",
  "created_at": "2024-01-15T10:02:00Z"
}
```

**Errors**

| Status | Code | Reason |
|---|---|---|
| 400 | `INVALID_WALLET_ID` | Malformed UUID |
| 400 | `INVALID_AMOUNT` | Non-positive or unparseable amount |
| 404 | `WALLET_NOT_FOUND` | Wallet does not exist |
| 409 | `TRANSACTION_EXISTS` | Reference already used on this wallet |
| 422 | `INSUFFICIENT_FUNDS` | Balance would go negative |

---

### Get balance

```
GET /wallets/{id}
```

**Response** `200 OK`
```json
{
  "id": "a1b2c3d4-...",
  "owner_id": "user-123",
  "balance": "60.00",
  "created_at": "2024-01-15T10:00:00Z",
  "updated_at": "2024-01-15T10:02:00Z"
}
```

**Errors**

| Status | Code | Reason |
|---|---|---|
| 400 | `INVALID_WALLET_ID` | Malformed UUID |
| 404 | `WALLET_NOT_FOUND` | Wallet does not exist |

---

### List transactions

```
GET /wallets/{id}/transactions?limit=50&cursor=<cursor>
```

Results are ordered newest first. Pass the `next_cursor` from each response as the `cursor` parameter on the next request to page through the full history.

**Query parameters**

| Parameter | Type | Default | Description |
|---|---|---|---|
| `limit` | integer | `50` | Number of results (1–100) |
| `cursor` | opaque string | — | Cursor token from previous page's `next_cursor` |

**Response** `200 OK`
```json
{
  "data": [
    {
      "id": "txn-uuid-...",
      "wallet_id": "a1b2c3d4-...",
      "type": "withdraw",
      "amount": "40.00",
      "balance_after": "60.00",
      "reference": "txn-xyz-456",
      "created_at": "2024-01-15T10:02:00Z"
    }
  ],
  "next_cursor": "eyJiZWZvcmUiOiIyMDI0LTAxLTE1VDEwOjAxOjAwWiIsImJlZm9yZV9pZCI6Ii4uLiJ9"
}
```

`next_cursor` is omitted when there are no more pages.

**Errors**

| Status | Code | Reason |
|---|---|---|
| 400 | `INVALID_WALLET_ID` | Malformed UUID |
| 400 | `INVALID_LIMIT` | Limit out of range |
| 400 | `INVALID_CURSOR` | Unparseable cursor token |
| 404 | `WALLET_NOT_FOUND` | Wallet does not exist |

---

### Error response shape

All errors follow this structure:

```json
{
  "status": 422,
  "code": "INSUFFICIENT_FUNDS",
  "message": "insufficient funds"
}
```

---

## Development

```bash
make test      # run all tests
make lint      # run golangci-lint
make fmt       # format code
make migrate   # apply pending migrations
make logs      # follow container logs
make clean     # remove containers and volumes
```

### Integration tests

Integration tests require a running Postgres instance:

```bash
TEST_DATABASE_URL=postgres://user:password@localhost:5434/wallet_db?sslmode=disable go test ./internal/infrastructure/postgres/...
```

## Project structure

```
.
├── cmd/server/          # entrypoint — wiring and server lifecycle
├── internal/
│   ├── application/     # use cases (deposit, withdraw, etc.)
│   ├── domain/          # entities, money type, ports, errors
│   ├── delivery/http/   # handlers, router, middleware, request/response types
│   └── infrastructure/
│       └── postgres/    # repository, transactor, migrations
```
