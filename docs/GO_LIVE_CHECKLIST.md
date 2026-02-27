# MMBot Go-Live Checklist

## Current Baseline

Use this exact local baseline:

1. Backend URL: `http://127.0.0.1:18080`
2. EA account id: `paper-1`
3. API auth mode: `OPENAI_API_KEY` in `.env`
4. EA defaults: `PollIntervalSeconds=5`, `SyncEveryLoops=10`

## Pre-Go-Live Checks

1. Start backend:
   - `go run ./cmd/server`
2. Compile and attach `ea/MMBotEA.mq5` in MT5.
3. MT5 `WebRequest` allowlist contains `http://127.0.0.1:18080`.
4. Run smoke:
   - `./scripts/paper-smoke.ps1 -BaseUrl "http://127.0.0.1:18080" -QueueSignal $true`
5. Confirm logs show:
   - `/admin/strategy/evaluate` `200` with command queued.
   - `/ea/execute` response size increases (OPEN command payload).
   - `/ea/result` `200`.

## Safety Guardrails (Must Stay On)

1. `MAX_DAILY_LOSS_PCT`
2. `MAX_OPEN_POSITIONS`
3. `MAX_SPREAD_PIPS`
4. `STRATEGY_RATE_LIMIT_PER_MIN`
5. `STRATEGY_MIN_INTERVAL`
6. `STRATEGY_DEDUP_TTL`
7. `STRATEGY_DAILY_BUDGET`
8. `STRATEGY_MAX_CANDLES`

## Session Checklist

Before each session:

1. Verify `/health` is `ok`.
2. Verify EA heartbeat continues every poll interval.
3. Verify `paused=false` unless intentionally paused.

After each session:

1. Check `/events?limit=50` for:
   - `RiskTriggered`
   - `OpenClawDeliveryFailed`
2. Check `/dashboard/summary?account_id=paper-1`.
3. Archive key logs if any trade execution failed.

## Stop Conditions

Pause immediately if any of these occur:

1. Repeated `/ea/result` failures.
2. Unexpected burst of queued OPEN commands.
3. `daily_loss_pct` approaches max threshold abnormally fast.
4. Any unknown `deny_reason` appears repeatedly.

