# Changelog

All notable changes to this project are documented in this file.

## [v0.1.0-paper] - 2026-02-27

### Added
- Go backend API with admin, EA, strategy, dashboard, events, telegram webhook, and health endpoints.
- MT5 EA real polling/execution loop in `ea/MMBotEA.mq5`.
- Paper smoke script with one-command signal queue option: `scripts/paper-smoke.ps1`.
- Paper validation and operational docs:
  - `docs/PAPER_MODE_RUNBOOK.md`
  - `docs/GO_LIVE_CHECKLIST.md`
- Strategy usage guardrails to reduce abuse/cost:
  - rate limit per minute
  - minimum interval cooldown
  - duplicate request suppression
  - daily strategy budget
  - max candles cap
- E2E tests for paper flow, API-key mode, circuit breaker, and strategy guardrails.
- `.env` autoload on backend startup for local runs.

### Changed
- Default backend listen address to `:18080`.
- Default EA base URL to `http://127.0.0.1:18080`.
- Safer EA polling defaults (`PollIntervalSeconds=5`, `SyncEveryLoops=10`).
- Provider auth mode supports `OPENAI_API_KEY` fast path (OAuth optional).

### Fixed
- MQL5 strict-profile compile issues in `ea/MMBotEA.mq5` (`StringUpper` compatibility and char conversion warnings).
- OpenClaw delivery reliability with retries and idempotency.

