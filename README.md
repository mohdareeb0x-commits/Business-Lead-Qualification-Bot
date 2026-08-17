# Telegram Lead Qualification & Booking Bot (MVP)

A production-quality Go backend that runs a Telegram bot to qualify leads
(real-estate in the MVP), persists them to MongoDB, and exposes a small
admin REST API for follow-up. Built as a single-binary service with
Docker / Docker Compose for deployment.

---

## 1. Architecture

```
                    ┌───────────────────────────────┐
                    │         Telegram Cloud         │
                    │  (sends updates via webhook)   │
                    └────────────────┬──────────────┘
                                     │ POST /api/v1/webhooks/telegram
                                     ▼
        ┌────────────────────────────────────────────────────────┐
        │                       Gin HTTP                          │
        │  /health  /ready  /api/v1/leads ...  /api/v1/dashboard  │
        └────┬───────────────────────────────────────────────┬────┘
             │                                               │
             ▼                                               ▼
   ┌──────────────────┐                            ┌────────────────────┐
   │  bot.Handler     │                            │  lead.Handler      │
   │ (state machine)  │                            │  (REST CRUD)       │
   └────┬─────────────┘                            └────────┬───────────┘
        │                                                   │
        └──────────────► lead.Service ◄────────────────────┘
                            │       │
                ┌───────────┘       └──────────────┐
                ▼                                  ▼
      ┌──────────────────┐                ┌──────────────────────┐
      │ lead.Repository  │                │ notification.Telegram│
      │   (Mongo driver) │                │ (admin chat via API) │
      └────┬─────────────┘                └──────────────────────┘
           ▼
      ┌──────────────────┐
      │    MongoDB       │
      │ telegram_leads   │
      │  - leads         │
      │  - lead_events   │
      └──────────────────┘
```

The layers communicate through interfaces (`lead.Repository`,
`lead.Notifier`, `lead.Scorer`, `bot.Store`) so each piece can be
replaced without touching the others. Redis, RabbitMQ, AI, or
microservices can be added by introducing new implementations of these
interfaces.

---

## 2. Tech stack

| Layer        | Choice                                        |
| ------------ | --------------------------------------------- |
| Language     | Go 1.22                                       |
| HTTP         | [Gin](https://github.com/gin-gonic/gin)       |
| Database     | MongoDB (official Go driver)                  |
| Telegram     | Native Bot HTTP API (no third-party client)   |
| Config       | `github.com/joho/godotenv` + env vars         |
| Logging      | `log/slog` (structured JSON)                  |
| Container    | Docker (multi-stage, distroless runtime)      |

---

## 3. Project structure

```
telegram-lead-bot/
├── cmd/server/main.go                # entrypoint
├── internal/
│   ├── apierr/                       # consistent error envelope
│   ├── bot/                          # Telegram bot
│   │   ├── bot.go                    # Gin route + webhook secret
│   │   ├── conversation.go           # state machine + Store interface
│   │   ├── handler.go                # update dispatch + send helpers
│   │   └── keyboard.go               # inline / reply keyboards
│   ├── config/                       # env loading
│   ├── database/                     # Mongo client + indexes
│   ├── health/                       # /health, /ready
│   ├── lead/                         # domain
│   │   ├── model.go                  # Lead, LeadEvent, enums
│   │   ├── dto.go                    # request/response + validation
│   │   ├── repository.go             # Mongo persistence
│   │   ├── service.go                # business logic
│   │   ├── handler.go                # REST handlers
│   │   └── validate.go               # phone normalization/validation
│   ├── middleware/                   # request-id, logger, recovery, rate limit
│   ├── notification/                 # admin Telegram notifier
│   └── scoring/                      # deterministic scoring rules
├── tests/                            # unit tests
├── Dockerfile
├── docker-compose.yml
├── .env.example
├── go.mod / go.sum
└── README.md
```

---

## 4. Environment variables

| Variable                 | Required | Default                 | Notes                                              |
| ------------------------ | -------- | ----------------------- | -------------------------------------------------- |
| `MONGODB_URI`            | yes      | —                       | e.g. `mongodb://mongodb:27017`                     |
| `MONGODB_DATABASE`       | no       | `telegram_leads`        |                                                    |
| `TELEGRAM_BOT_TOKEN`     | yes*     | —                       | required to send admin notifications + webhooks    |
| `TELEGRAM_ADMIN_CHAT_ID` | yes      | —                       | integer; receives every new lead                   |
| `TELEGRAM_WEBHOOK_SECRET`| no       | (empty)                 | if set, validated via `X-Telegram-Bot-Api-Secret-Token` |
| `SERVER_PORT`            | no       | `8080`                  |                                                    |
| `SERVER_ENV`             | no       | `development`           |                                                    |
| `GIN_MODE`               | no       | `release`               | `debug` / `release` / `test`                      |

> \* `TELEGRAM_BOT_TOKEN` is only checked at startup; an empty value logs a
> warning and the webhook still returns 200 OK without forwarding. Set the
> secret via environment only — do not commit `.env`.

---

## 5. Local setup (without Docker)

```bash
# 1. Install deps
go mod tidy

# 2. Copy env and fill in your values
cp .env.example .env

# 3. Make sure MongoDB is reachable at $MONGODB_URI
#    (e.g. `docker run -p 27017:27017 mongo:7`)

# 4. Run the server
go run ./cmd/server
```

---

## 6. Docker setup

```bash
# build + start app + mongodb
docker compose up --build

# tail logs
docker compose logs -f app
```

The image is multi-stage: build on `golang:1.22-alpine`, run on
`gcr.io/distroless/static-debian12:nonroot`. The MongoDB service uses a
named volume `mongo_data` for persistence and is **not** published on the
host.

---

## 7. Telegram bot setup

1. Talk to [@BotFather](https://t.me/BotFather), create a bot, copy the
   token into `TELEGRAM_BOT_TOKEN`.
2. Get your personal chat id (e.g. via [@userinfobot](https://t.me/userinfobot))
   and put it in `TELEGRAM_ADMIN_CHAT_ID`.
3. Pick a strong random secret for `TELEGRAM_WEBHOOK_SECRET` (e.g. `openssl rand -hex 32`).
4. Make the app reachable over HTTPS. The webhook URL is:
   `https://<your-domain>/api/v1/webhooks/telegram`.

Register the webhook with Telegram's Bot API:

```bash
curl -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/setWebhook" \
     -H "Content-Type: application/json" \
     -d '{
       "url": "https://<your-domain>/api/v1/webhooks/telegram",
       "secret_token": "<TELEGRAM_WEBHOOK_SECRET>",
       "allowed_updates": ["message", "callback_query"]
     }'
```

Verify:

```bash
curl "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/getWebhookInfo"
```

---

## 8. API endpoints

| Method | Path                                | Description                       |
| ------ | ----------------------------------- | --------------------------------- |
| GET    | `/health`                           | liveness                          |
| GET    | `/ready`                            | readiness (pings Mongo)           |
| GET    | `/api/v1/leads`                     | list, filter, paginate            |
| GET    | `/api/v1/leads/:id`                 | fetch one                         |
| PATCH  | `/api/v1/leads/:id/status`          | change status                     |
| GET    | `/api/v1/leads/:id/events`          | lead audit history                |
| GET    | `/api/v1/dashboard/stats`           | counters by status / temperature  |
| POST   | `/api/v1/webhooks/telegram`         | Telegram webhook (secret-checked) |

### `GET /api/v1/leads`

Query params:

- `page` (default `1`)
- `limit` (default `20`, max `100`)
- `status` (one of `NEW`, `CONTACTED`, `QUALIFIED`, `CONVERTED`, `LOST`)
- `temperature` (one of `HOT`, `WARM`, `COLD`)

Response:

```json
{
  "data": [ { "id": "65f...", "name": "Ahmed", "score": 92, "temperature": "HOT", "status": "NEW" } ],
  "pagination": { "page": 1, "limit": 20, "total": 1, "total_pages": 1 }
}
```

### `PATCH /api/v1/leads/:id/status`

```json
{ "status": "CONTACTED" }
```

Validates the new status, updates the lead, records a
`STATUS_CHANGED` event with `{ "from": "NEW", "to": "CONTACTED" }` in
metadata. Returns the updated lead.

### `GET /api/v1/dashboard/stats`

```json
{
  "total_leads": 142,
  "by_status":     { "NEW": 50, "CONTACTED": 30, "QUALIFIED": 20, "CONVERTED": 12, "LOST": 30 },
  "by_temperature": { "HOT": 25, "WARM": 60, "COLD": 57 },
  "leads_created_today": 7
}
```

> **Note:** the admin REST API is currently **unauthenticated**. Do not
> expose it to the public internet without putting it behind a reverse
> proxy, VPN, or a future auth layer (JWT, mTLS, etc.).

---

## 9. Lead scoring rules

Implemented in `internal/scoring/scorer.go` as a pure function over a
small `Input` struct so the rules can change without touching the
Telegram bot or repository.

| Field      | Rule                                   | Points |
| ---------- | -------------------------------------- | -----: |
| Budget     | `>= 10,000,000`                        |    +30 |
|            | `>= 5,000,000`                         |    +20 |
|            | otherwise                              |    +10 |
| Timeline   | `Immediately`                          |    +30 |
|            | `1-3 months`                           |    +20 |
|            | `3-6 months`                           |    +10 |
|            | `Just researching`                     |     +5 |
| Location   | any non-empty value                    |    +20 |
|            | empty                                  |     +0 |
| Phone      | valid phone number                     |    +10 |

Score is clamped to `[0, 100]`. Temperature:

- `80–100` → **HOT**
- `50–79` → **WARM**
- `0–49` → **COLD**

The free-form budget parser extracts the first run of digits, so
`₹90L`, `1.5 Cr`, and `5000000` all parse.

---

## 10. Telegram conversation flow

The bot keeps per-user state in memory (`internal/bot/conversation.go`)
keyed by Telegram user id. The `Store` interface lets you swap in Redis
later.

```
/start
  ↓
ASK_SERVICE            ← inline buttons: Buy / Rent / Sell
  ↓
ASK_PROPERTY_TYPE      ← inline buttons: Apartment / House / Villa / Commercial
  ↓
ASK_BUDGET             ← free text (e.g. "₹90L")
  ↓
ASK_LOCATION           ← free text
  ↓
ASK_TIMELINE           ← inline buttons: Immediately / 1-3 mo / 3-6 mo / Just researching
  ↓
ASK_NAME               ← free text
  ↓
ASK_PHONE              ← reply keyboard: "Share phone number" (request_contact)
  ↓
CONFIRM                ← inline buttons: Confirm / Edit / Cancel
  ↓
COMPLETED              → lead is created, scored, saved, event logged, admin notified
```

`Edit` resets the conversation to `ASK_SERVICE`. `Cancel` deletes the
state. Idle conversations expire after 30 minutes.

---

## 11. MongoDB

Database: `telegram_leads`

Collections:

- `leads` — one document per qualified lead
- `lead_events` — append-only audit log (creation, scoring, status changes)

Indexes (auto-created on startup):

- `leads`: `telegram_id`, `status`, `score` (desc), `created_at` (desc)
- `lead_events`: `lead_id`, `created_at` (desc)

---

## 12. Security

- Request body limited to 1 MiB on the webhook.
- Telegram webhook secret checked with `crypto/subtle.ConstantTimeCompare`.
- Panic recovery returns a generic 500 envelope; stack traces stay in
  server logs.
- Structured JSON logs include method, path, status, duration, request id.
  They do **not** include the bot token, the webhook secret, or the
  full request body (so phone numbers never appear in logs).
- CORS is permissive by default for local development; restrict
  `AllowOrigins` in production.
- Simple per-IP token-bucket rate limiter on the public API.
- **Admin REST API has no authentication yet** — gate it behind a proxy
  or add auth before exposing it publicly.

---

## 13. Running tests

```bash
go test ./...
```

Tests cover scoring rules, status-transition validation, conversation
state transitions, phone validation, and DTO validation.

---

## 14. Graceful shutdown

On `SIGINT` / `SIGTERM` the server:

1. Stops accepting new HTTP connections (`http.Server.Shutdown`).
2. Lets in-flight handlers finish (10s timeout).
3. Disconnects from MongoDB.

---

## 15. Future improvements

- Redis-backed `bot.Store` so conversations survive restarts and scale
  horizontally.
- Authentication for `/api/v1` (JWT or mTLS).
- Per-tenant configuration of questionnaire + scoring rules so the same
  binary can serve real-estate, education, healthcare, etc.
- Lead deduplication by `telegram_id` + service.
- AI-assisted free-text requirement parsing.
- Webhook delivery observability (retry counters, dead-letter table).
- Outbound email / SMS notifications alongside Telegram.
