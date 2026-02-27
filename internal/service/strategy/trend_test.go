package strategy

import (
	"testing"
	"time"
)

func TestTrendEngineBuySignal(t *testing.T) {
	engine := NewTrendEngine()
	candles := make([]Candle, 0, 120)
	base := 1.0800
	for i := 0; i < 120; i++ {
		// Gradual uptrend with small pullbacks.
		step := float64(i) * 0.00035
		close := base + step
		if i%7 == 0 {
			close -= 0.0002
		}
		open := close - 0.0001
		high := close + 0.0004
		low := close - 0.0005
		candles = append(candles, Candle{
			Time:  time.Unix(int64(1700000000+i*900), 0).UTC(),
			Open:  open,
			High:  high,
			Low:   low,
			Close: close,
		})
	}

	sig, err := engine.Evaluate(TrendInput{
		Symbol:  "EURUSD",
		Candles: candles,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sig.HasSignal {
		t.Fatalf("expected signal, got none: %+v", sig)
	}
	if sig.Side != "BUY" {
		t.Fatalf("expected BUY signal, got %s", sig.Side)
	}
	if sig.Confidence < 0.55 || sig.Confidence > 0.90 {
		t.Fatalf("unexpected confidence %.4f", sig.Confidence)
	}
}

func TestTrendEngineNoSignalOnFlatSeries(t *testing.T) {
	engine := NewTrendEngine()
	candles := make([]Candle, 0, 120)
	base := 1.1000
	for i := 0; i < 120; i++ {
		close := base
		if i%2 == 0 {
			close += 0.00005
		} else {
			close -= 0.00005
		}
		candles = append(candles, Candle{
			Time:  time.Unix(int64(1700000000+i*900), 0).UTC(),
			Open:  close,
			High:  close + 0.0002,
			Low:   close - 0.0002,
			Close: close,
		})
	}

	sig, err := engine.Evaluate(TrendInput{
		Symbol:  "EURUSD",
		Candles: candles,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig.HasSignal {
		t.Fatalf("expected no signal, got %+v", sig)
	}
}
