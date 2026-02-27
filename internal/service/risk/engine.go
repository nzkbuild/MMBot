package risk

import (
	"strings"

	"mmbot/internal/domain"
)

type Engine struct {
	maxOpenPositions int
	maxDailyLossPct float64
	minConfidence   float64
	maxSpreadPips   float64
}

func NewEngine(maxOpenPositions int, maxDailyLossPct, minConfidence, maxSpreadPips float64) *Engine {
	return &Engine{
		maxOpenPositions: maxOpenPositions,
		maxDailyLossPct:  maxDailyLossPct,
		minConfidence:    minConfidence,
		maxSpreadPips:    maxSpreadPips,
	}
}

func (e *Engine) Evaluate(input domain.SignalInput, state domain.StrategyState) domain.RiskDecision {
	if state.Paused {
		return domain.RiskDecision{Allowed: false, DenyReason: "bot_paused"}
	}
	if strings.TrimSpace(input.Symbol) == "" {
		return domain.RiskDecision{Allowed: false, DenyReason: "symbol_missing"}
	}
	if strings.TrimSpace(input.Side) == "" {
		return domain.RiskDecision{Allowed: false, DenyReason: "side_missing"}
	}
	if input.StopLossPips <= 0 {
		return domain.RiskDecision{Allowed: false, DenyReason: "stop_loss_required"}
	}
	if input.SpreadPips > e.maxSpreadPips {
		return domain.RiskDecision{Allowed: false, DenyReason: "spread_too_high"}
	}
	if input.Confidence < e.minConfidence {
		return domain.RiskDecision{Allowed: false, DenyReason: "ai_confidence_too_low"}
	}
	if state.OpenPositions >= e.maxOpenPositions {
		return domain.RiskDecision{Allowed: false, DenyReason: "max_open_positions_reached"}
	}
	if state.DailyLossPct >= e.maxDailyLossPct {
		return domain.RiskDecision{Allowed: false, DenyReason: "daily_loss_limit_hit"}
	}
	return domain.RiskDecision{Allowed: true}
}

