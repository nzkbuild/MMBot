package risk

import (
	"testing"

	"mmbot/internal/domain"
)

func TestEvaluate_AllowsValidSignal(t *testing.T) {
	engine := NewEngine(3, 2.0, 0.70, 2.0)
	decision := engine.Evaluate(
		domain.SignalInput{
			Symbol:         "EURUSD",
			Side:           "BUY",
			Confidence:     0.82,
			SpreadPips:     1.2,
			StopLossPips:   12,
			TakeProfitPips: 24,
		},
		domain.StrategyState{Paused: false, OpenPositions: 1, DailyLossPct: 0.4},
	)
	if !decision.Allowed {
		t.Fatalf("expected allowed decision, got deny=%q", decision.DenyReason)
	}
}

func TestEvaluate_RejectsLowConfidence(t *testing.T) {
	engine := NewEngine(3, 2.0, 0.70, 2.0)
	decision := engine.Evaluate(
		domain.SignalInput{
			Symbol:       "EURUSD",
			Side:         "BUY",
			Confidence:   0.6,
			SpreadPips:   1.0,
			StopLossPips: 10,
		},
		domain.StrategyState{},
	)
	if decision.Allowed || decision.DenyReason != "ai_confidence_too_low" {
		t.Fatalf("expected ai_confidence_too_low, got %+v", decision)
	}
}

func TestEvaluate_RejectsPaused(t *testing.T) {
	engine := NewEngine(3, 2.0, 0.70, 2.0)
	decision := engine.Evaluate(
		domain.SignalInput{
			Symbol:       "EURUSD",
			Side:         "SELL",
			Confidence:   0.9,
			SpreadPips:   0.8,
			StopLossPips: 10,
		},
		domain.StrategyState{Paused: true},
	)
	if decision.Allowed || decision.DenyReason != "bot_paused" {
		t.Fatalf("expected bot_paused, got %+v", decision)
	}
}

