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
| Pagination | Cursor-based (`before` timestamp) — safe under concurrent inserts |
| Logging | Structured JSON via `slog` — mutations only, not reads |
| Shutdown | Graceful — drains in-flight requests before closing (10s timeout) |

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

Copy `.env.example` to `.env` and adjust as needed.

## API

Base URL: `http://localhost:8080`

All request and response bodies are JSON. Money values are strings to preserve decimal precision.

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

> `reference` is optional. If provided and non-empty, repeated requests with the same reference on the same wallet return `409 TRANSACTION_EXISTS` — use this for safe retries.

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
GET /wallets/{id}/transactions?limit=50&before=<cursor>
```

**Query parameters**

| Parameter | Type | Default | Description |
|---|---|---|---|
| `limit` | integer | `50` | Number of results (1–100) |
| `before` | RFC3339 timestamp | — | Cursor from previous page's `next_cursor` |

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
  "next_cursor": "2024-01-15T10:01:00Z"
}
```

> `next_cursor` is omitted when there are no more pages. Pass it as `before` to fetch the next page.

**Errors**

| Status | Code | Reason |
|---|---|---|
| 400 | `INVALID_WALLET_ID` | Malformed UUID |
| 400 | `INVALID_LIMIT` | Limit out of range |
| 400 | `INVALID_CURSOR` | Unparseable `before` timestamp |
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
