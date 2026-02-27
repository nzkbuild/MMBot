# MMBot MVP

AI-assisted trading autopilot MVP:
- MT5 EA executes broker actions.
- Go backend handles strategy/risk/orchestration.
- OpenAI provider auth supports API key mode (`OPENAI_API_KEY`) and OAuth mode.
- Telegram provides alerts/control.
- OpenClaw receives outbound events.

## Current Implementation Status

This repository now includes:
1. Go HTTP backend scaffold with all MVP core endpoints.
2. PostgreSQL-backed runtime store (commands/events/risk state/provider connection) with memory fallback.
3. PostgreSQL schema migrations (`migrations/0001_init.sql`, `migrations/0002_runtime_state.sql`).
4. OpenAI provider auth with API key fast-path and OAuth authorization-code exchange + refresh fallback.
5. Encrypted token-at-rest storage for OAuth provider access/refresh tokens (AES-GCM).
6. Real MT5 EA polling/execution implementation (`ea/MMBotEA.mq5`).
7. Hardened OpenClaw delivery with retries and idempotency headers.
4. Risk engine with hard safety rules and unit tests.
5. Telegram notifier integration.
6. OpenClaw outbound webhook publisher.
7. MT5 EA example skeleton (`ea/MMBotEA.example.mq5`).

## API Summary

### Public
- `POST /admin/login`
- `POST /ea/register`
- `POST /telegram/webhook`
- `GET /oauth/openai/start`
- `GET /oauth/openai/callback`
- `GET /health`

### Admin auth required
- `POST /admin/logout`
- `POST /bot/pause`
- `POST /bot/resume`
- `GET /dashboard/summary`
- `GET /events`
- `GET /oauth/openai/status`
- `POST /oauth/openai/disconnect`
- `POST /admin/signals/evaluate`
- `POST /admin/strategy/evaluate`

### EA auth required
- `POST /ea/heartbeat`
- `POST /ea/sync`
- `POST /ea/execute`
- `POST /ea/result`

`/ea/sync` behavior:
1. Stores raw snapshot payload.
2. Derives open position count and daily loss % from payload fields.
3. Updates runtime risk state (`open_positions`, `daily_loss_pct`).
4. Triggers pause circuit breaker if `daily_loss_pct >= MAX_DAILY_LOSS_PCT`.

## Safety Rules Enforced

The risk engine blocks new opens when:
1. Bot is paused.
2. Stop-loss is missing.
3. Spread exceeds configured cap.
4. AI confidence is below threshold.
5. Max open positions is reached.
6. Daily loss limit is reached.
7. Strategy usage guardrails trigger (`strategy_rate_limit_exceeded`, `strategy_cooldown_active`, `strategy_duplicate_request`, `strategy_daily_budget_exceeded`).

## Local Configuration

Copy `.env.example` and set values:

```bash
cp .env.example .env
```

Important variables:
- `STORE_MODE`, `DATABASE_URL`
- `OAUTH_ENCRYPTION_KEY` (base64-encoded 32-byte key)
- `ADMIN_USERNAME`, `ADMIN_PASSWORD`, `JWT_SECRET`
- `EA_CONNECT_CODE`, `EA_TOKEN_TTL`
- `AI_MIN_CONFIDENCE`, `MAX_DAILY_LOSS_PCT`, `MAX_OPEN_POSITIONS`, `MAX_SPREAD_PIPS`
- `STRATEGY_RATE_LIMIT_PER_MIN`, `STRATEGY_MIN_INTERVAL`, `STRATEGY_DEDUP_TTL`, `STRATEGY_DAILY_BUDGET`, `STRATEGY_MAX_CANDLES`
- `OPENAI_API_KEY` (recommended)
- `OPENAI_CLIENT_ID`, `OPENAI_CLIENT_SECRET`, `OPENAI_AUTH_URL`, `OPENAI_TOKEN_URL`, `OPENAI_SCOPES`, `OPENAI_REDIRECT_URI`, `OPENAI_REFRESH_SKEW` (optional OAuth mode)
- `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`
- `OPENCLAW_WEBHOOK_URL`
- `OPENCLAW_TIMEOUT`, `OPENCLAW_MAX_RETRIES`, `OPENCLAW_RETRY_BASE`, `OPENCLAW_RETRY_MAX`

## Run PostgreSQL

```bash
docker compose up -d postgres
```

The migration SQL is mounted to `docker-entrypoint-initdb.d` and auto-applies on first init.

## Run Backend

```bash
go run ./cmd/server
```

Paper-mode validation runbook:
- [PAPER_MODE_RUNBOOK.md](C:/Users/nbzkr/OneDrive/Documents/Coding/MMBot/docs/PAPER_MODE_RUNBOOK.md)

## MT5 EA (Real Loop)

Compile and attach:
1. Open MetaEditor.
2. Open [MMBotEA.mq5](C:/Users/nbzkr/OneDrive/Documents/Coding/MMBot/ea/MMBotEA.mq5).
3. Compile to `.ex5`.
4. Attach EA to a chart in MT5.

Required MT5 setting:
1. In MT5, go to `Tools -> Options -> Expert Advisors`.
2. Enable `Allow WebRequest for listed URL`.
3. Add your backend base URL (example: `http://127.0.0.1:8080`).

EA behavior:
1. Registers with `/ea/register` using connect code.
2. Sends `/ea/heartbeat`.
3. Sends `/ea/sync` snapshots with positions + PnL metrics.
4. Polls `/ea/execute`.
5. Executes command types (`OPEN`, `CLOSE`, `MOVE_SL`, `SET_TP`, `PAUSE`, `RESUME`).
6. Reliably reports `/ea/result` with pending retry on network failures.

## Quick Manual Flow

1. `POST /admin/login` to get admin JWT.
2. Configure provider auth:
   - Recommended: set `OPENAI_API_KEY` and skip OAuth routes.
   - Optional OAuth: `GET /oauth/openai/start`, then call `/oauth/openai/callback` with `state` + `code`.
3. `POST /ea/register` using connect code.
4. `POST /admin/strategy/evaluate` with M15 candles payload.
5. EA polls `/ea/execute`, performs action, posts `/ea/result`.
6. Check `/dashboard/summary` and `/events`.

## Notes

1. If `OPENAI_API_KEY` is set, provider is considered connected and OAuth routes become optional no-ops.
2. In OAuth mode, callback performs real code exchange at configured token endpoint.
3. In OAuth mode, token refresh is attempted automatically before expiry (`OPENAI_REFRESH_SKEW` window).
4. In Postgres mode, OAuth provider tokens are encrypted at rest with `OAUTH_ENCRYPTION_KEY`.
4. Set `STORE_MODE=postgres` + valid `DATABASE_URL` to use persistent runtime state.
5. OpenClaw uses `X-Idempotency-Key` = event ID and retries failed deliveries with exponential backoff.
6. Failed OpenClaw deliveries are logged and appended as `OpenClawDeliveryFailed` events.
7. Default mode is paper/sim semantics.

## Strategy Endpoint Payload

`POST /admin/strategy/evaluate`

```json
{
  "account_id": "paper-1",
  "symbol": "EURUSD",
  "spread_pips": 1.2,
  "candles": [
    {
      "time": "2026-02-27T14:00:00Z",
      "open": 1.0821,
      "high": 1.0829,
      "low": 1.0817,
      "close": 1.0826
    }
  ]
}
```

Expected behavior:
1. Trend engine evaluates EMA20/EMA50 + ATR from provided candles.
2. If no trend setup, returns `has_signal=false`.
3. If setup exists, risk/AI gate runs and command is queued for EA polling.

## Telegram Commands (Webhook)

Webhook endpoint:
- `POST /telegram/webhook`

Supported commands from allowed chats:
1. `/pause`
2. `/resume`
3. `/today [account_id]`
4. `/help`

Relevant env vars:
1. `TELEGRAM_BOT_TOKEN`
2. `TELEGRAM_CHAT_ID` (default trusted chat)
3. `TELEGRAM_ALLOWED_CHAT_IDS` (comma-separated extra trusted chat IDs)
4. `TELEGRAM_WEBHOOK_SECRET` (validate `X-Telegram-Bot-Api-Secret-Token`)
