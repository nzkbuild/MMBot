# Paper Mode Runbook

## Purpose

Validate MMBot end-to-end in paper mode and provide incident-response steps for common failures.

## Preconditions

1. Go 1.22+ installed.
2. `.env` configured from `.env.example`.
3. OpenAI provider auth configured:
   - preferred: `OPENAI_API_KEY`, or
   - optional: OAuth client settings + callback flow.
4. Backend started (`go run ./cmd/server`).
5. MT5 terminal configured with `WebRequest` allowlist including backend URL.

## Automated Validation

### 1) Full test/build gate

```powershell
$env:GOCACHE="$PWD\.cache\go-build"
$env:GOPATH="$PWD\.cache\gopath"
$env:GOMODCACHE="$PWD\.cache\gomod"
go test ./...
go build ./...
```

### 2) Paper smoke API checks

```powershell
./scripts/paper-smoke.ps1 `
  -BaseUrl "http://127.0.0.1:8080" `
  -AdminUsername "admin" `
  -AdminPassword "change-me" `
  -ConnectCode "MMBOT-ONE-TIME-CODE" `
  -AccountId "paper-1" `
  -DeviceId "smoke-device-1"
```

Optional Telegram command check:

```powershell
./scripts/paper-smoke.ps1 `
  -BaseUrl "http://127.0.0.1:8080" `
  -AdminUsername "admin" `
  -AdminPassword "change-me" `
  -ConnectCode "MMBOT-ONE-TIME-CODE" `
  -AccountId "paper-1" `
  -DeviceId "smoke-device-1" `
  -TelegramChatId "<trusted-chat-id>" `
  -TelegramWebhookSecret "<secret-if-configured>"
```

### 3) Integration-level E2E tests

```powershell
go test ./internal/http -run E2E -v
```

## Manual MT5 Loop Validation

1. Compile and attach `ea/MMBotEA.mq5`.
2. Confirm logs show:
   - register success
   - heartbeat success
   - periodic sync
   - execute polling
3. Trigger a strategy command from backend (`/admin/strategy/evaluate`) and confirm EA reports `/ea/result`.
4. Confirm dashboard/event stream updates.

## Expected Green Signals

1. `/health` returns `status=ok`.
2. `/ea/heartbeat` returns `paused` flag and server time.
3. `/ea/sync` updates `open_positions` and `daily_loss_pct`.
4. `/admin/strategy/evaluate` can queue commands when provider/risk allow.
5. `/ea/execute` returns queued command or `NOOP`.
6. `/ea/result` logs `TradeExecuted` or `TradeModified`.
7. OpenClaw failures produce `OpenClawDeliveryFailed` events.

## Incident Playbook

### A) EA cannot reach backend

Checks:
1. MT5 `WebRequest` allowlist contains exact backend URL.
2. Backend process is running and reachable from MT5 host.
3. Token not expired/rejected (`/ea/heartbeat` 401 implies re-register flow).

Actions:
1. Re-run `/ea/register` flow by resetting EA token.
2. Validate backend logs for rejected requests.

### B) Bot remains paused unexpectedly

Checks:
1. `/events` for `BotPaused` and `RiskTriggered`.
2. `/dashboard/summary` `daily_loss_pct` vs `MAX_DAILY_LOSS_PCT`.

Actions:
1. Use `/resume` (admin or Telegram) after confirming risk condition has cleared.
2. If daily loss still high, keep paused and inspect PnL source fields in `/ea/sync`.

### C) OpenClaw delivery errors

Checks:
1. `/events` for `OpenClawDeliveryFailed`.
2. OpenClaw endpoint health and auth expectations.

Actions:
1. Verify `OPENCLAW_WEBHOOK_URL`.
2. Increase retries/timeouts if transient network issues are frequent.

### D) Telegram command webhook ignored

Checks:
1. `TELEGRAM_WEBHOOK_SECRET` header matches config.
2. Chat ID appears in `TELEGRAM_ALLOWED_CHAT_IDS` or matches `TELEGRAM_CHAT_ID`.

Actions:
1. Correct webhook secret and allowlist values.
2. Re-send `/help` from trusted chat to verify command path.
