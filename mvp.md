An **AI-assisted trading autopilot** where **MT5 EA executes trades**, while a **Go backend + AI decides/controls**, and users manage everything via **OAuth + Telegram + OpenClaw**.

---

## 2) Simple architecture (who does what)

### A) MT5 (EA) — “Hands”

* Runs inside MT5.
* Only does: **place/modify/close orders**, set SL/TP, manage positions.
* Talks to your backend via **HTTPS** (WebRequest) or a local bridge.

### B) Go Backend — “Brain + Rules”

* Auth, user accounts, broker account linking.
* Strategy engine + risk rules.
* Sends commands to EA: `OPEN`, `CLOSE`, `MOVE_SL`, `SET_TP`, `PAUSE`, etc.
* Logs everything (audit trail).

### C) AI — “Advisor”

* Doesn’t directly click trades.
* Produces: **signal/confidence**, market regime, or “hold/exit/adjust” suggestions.
* Your backend **wraps AI output with hard risk rules**.

### D) Telegram Bot — “Remote control”

* Start/stop bot, status, alerts, approvals (optional).
* “/pause”, “/closeall”, “/risk 0.5%”, “/today”

### E) OpenClaw — “Your orchestrator layer”

* Triggers workflows (cron, webhooks), routes events, connects other apps.
* Example: “If drawdown > X → send TG + pause EA + open incident ticket”.

---

## 3) User flow (OAuth v1 now, token later)

### OAuth v1 (Phase 1)

1. User signs up on your web app.
2. User connects Telegram (simple link code).
3. User installs EA on MT5 and pastes a **one-time connect code**.
4. EA registers device/session with backend.
5. Backend issues a **scoped session token** for EA.

### API Token (Phase 2)

* Replace connect code with long-lived **API token** + refresh / rotation.
* Add scopes: `trade:execute`, `trade:read`, `account:read`, `notifications:send`

Key idea: **EA should never hold OAuth access tokens.** EA holds **a limited bot token** only.

---

## 4) MVP scope (keep it small but real)

### MVP features

* 1 strategy mode (pick ONE):
  **A) Trend-following** or **B) Mean reversion** (don’t do both now)
* Basic risk:

  * fixed risk per trade (e.g., 0.5% equity)
  * max daily loss
  * max open positions
* Telegram:

  * trade opened/closed alerts
  * /pause and /resume
* Dashboard (simple):

  * account status connected/disconnected
  * last 20 actions + PnL summary
* AI role in MVP:

  * AI outputs only: **“trade allowed? yes/no + confidence + reason”**
  * Your rules still decide final execution

---

## 5) The “contracts” you need (so it’s buildable)

### EA ↔ Backend commands (minimal)

* `POST /ea/heartbeat`
* `POST /ea/sync` (positions/orders snapshot)
* `POST /ea/execute` (backend → EA via pull OR EA polls)
* `POST /ea/result` (EA reports action success/fail)

Start with **polling** (EA polls `/ea/execute` every 1–2s). It’s simpler than pushing.

### Event model (everything becomes an event)

* `SignalProposed`
* `TradeExecuted`
* `TradeModified`
* `RiskTriggered`
* `BotPaused`
  This makes OpenClaw + Telegram integration clean.

---

## 6) Safety rules (non-negotiable)

* AI suggestions **must be filtered** by rules:

  * position sizing limits
  * stop-loss required
  * spread/slippage sanity check
  * max drawdown circuit breaker
* “Kill switch” always available via Telegram and dashboard.

---

## 7) Your next 7 build steps (practical)

1. Define MVP strategy (trend or mean reversion) + 5 risk rules.
2. Build Go backend skeleton: auth, users, EA registration, event log.
3. Build EA: heartbeat + poll for commands + execute + report results.
4. Add Telegram bot: link account + /pause /resume + alerts.
5. Add AI layer as “advisor”: it only returns allow/deny + confidence.
6. Add OpenClaw hook: forward events to OpenClaw endpoint/workflow.
7. Add dashboard page: connection status + last events + toggles.

---