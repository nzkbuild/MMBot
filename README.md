# MMBot MVP

AI-assisted trading autopilot MVP:
- MT5 EA executes broker actions.
- Go backend handles strategy/risk/orchestration.
- OpenAI OAuth is used for provider authorization.
- Telegram provides alerts/control.
- OpenClaw receives outbound events.

## Current Implementation Status

This repository now includes:
1. Go HTTP backend scaffold with all MVP core endpoints.
2. PostgreSQL-backed runtime store (commands/events/risk state/OAuth connection) with memory fallback.
3. PostgreSQL schema migrations (`migrations/0001_init.sql`, `migrations/0002_runtime_state.sql`).
4. Risk engine with hard safety rules and unit tests.
5. Telegram notifier integration.
6. OpenClaw outbound webhook publisher.
7. MT5 EA example skeleton (`ea/MMBotEA.example.mq5`).

## API Summary

### Public
- `POST /admin/login`
- `POST /ea/register`
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

### EA auth required
- `POST /ea/heartbeat`
- `POST /ea/sync`
- `POST /ea/execute`
- `POST /ea/result`

## Safety Rules Enforced

The risk engine blocks new opens when:
1. Bot is paused.
2. Stop-loss is missing.
3. Spread exceeds configured cap.
4. AI confidence is below threshold.
5. Max open positions is reached.
6. Daily loss limit is reached.

## Local Configuration

Copy `.env.example` and set values:

```bash
cp .env.example .env
```

Important variables:
- `STORE_MODE`, `DATABASE_URL`
- `ADMIN_USERNAME`, `ADMIN_PASSWORD`, `JWT_SECRET`
- `EA_CONNECT_CODE`, `EA_TOKEN_TTL`
- `AI_MIN_CONFIDENCE`, `MAX_DAILY_LOSS_PCT`, `MAX_OPEN_POSITIONS`, `MAX_SPREAD_PIPS`
- `OPENAI_CLIENT_ID`, `OPENAI_REDIRECT_URI`
- `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`
- `OPENCLAW_WEBHOOK_URL`

## Run PostgreSQL

```bash
docker compose up -d postgres
```

The migration SQL is mounted to `docker-entrypoint-initdb.d` and auto-applies on first init.

## Run Backend

```bash
go run ./cmd/server
```

## Quick Manual Flow

1. `POST /admin/login` to get admin JWT.
2. `GET /oauth/openai/start`, then call `/oauth/openai/callback` with `state` + `code`.
3. `POST /ea/register` using connect code.
4. `POST /admin/signals/evaluate` with signal payload.
5. EA polls `/ea/execute`, performs action, posts `/ea/result`.
6. Check `/dashboard/summary` and `/events`.

## Notes

1. OAuth callback currently stores a placeholder provider token for MVP wiring. Replace with real token exchange in production.
2. Set `STORE_MODE=postgres` + valid `DATABASE_URL` to use persistent runtime state.
3. Default mode is paper/sim semantics.
