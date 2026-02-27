package strategy

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type Candle struct {
	Time   time.Time `json:"time"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume float64   `json:"volume,omitempty"`
}

type TrendInput struct {
	Symbol     string   `json:"symbol"`
	Candles    []Candle `json:"candles"`
	SpreadPips float64  `json:"spread_pips"`
}

type TrendSignal struct {
	HasSignal      bool    `json:"has_signal"`
	Side           string  `json:"side,omitempty"`
	Confidence     float64 `json:"confidence,omitempty"`
	Reason         string  `json:"reason,omitempty"`
	StopLossPips   float64 `json:"stop_loss_pips,omitempty"`
	TakeProfitPips float64 `json:"take_profit_pips,omitempty"`
}

type TrendEngine struct {
	FastEMA int
	SlowEMA int
	ATRLen  int
}

func NewTrendEngine() *TrendEngine {
	return &TrendEngine{
		FastEMA: 20,
		SlowEMA: 50,
		ATRLen:  14,
	}
}

func (e *TrendEngine) Evaluate(input TrendInput) (TrendSignal, error) {
	if strings.TrimSpace(input.Symbol) == "" {
		return TrendSignal{}, errors.New("symbol is required")
	}
	minCandles := max(e.SlowEMA+2, e.ATRLen+2)
	if len(input.Candles) < minCandles {
		return TrendSignal{}, fmt.Errorf("at least %d candles required", minCandles)
	}

	closes := make([]float64, 0, len(input.Candles))
	for _, c := range input.Candles {
		if c.Close <= 0 || c.High <= 0 || c.Low <= 0 {
			return TrendSignal{}, errors.New("candles must have positive prices")
		}
		closes = append(closes, c.Close)
	}

	fastNow := ema(closes, e.FastEMA)
	slowNow := ema(closes, e.SlowEMA)
	fastPrev := ema(closes[:len(closes)-1], e.FastEMA)
	slowPrev := ema(closes[:len(closes)-1], e.SlowEMA)
	lastClose := closes[len(closes)-1]
	atrNow := atr(input.Candles, e.ATRLen)
	trendGapPct := math.Abs(fastNow-slowNow) / slowNow
	fastSlopePct := math.Abs(fastNow-fastPrev) / fastNow

	// Ignore weak/flat regimes to reduce false positives.
	if trendGapPct < 0.0002 || fastSlopePct < 0.00005 {
		return TrendSignal{
			HasSignal: false,
			Reason:    "no clear trend setup",
		}, nil
	}

	// Buy trend regime: price above fast, fast above slow, and fast slope up.
	if lastClose > fastNow && fastNow > slowNow && fastNow > fastPrev && slowNow >= slowPrev {
		conf := confidence(lastClose, fastNow, slowNow, atrNow)
		slPips, tpPips := sltpPips(input.Symbol, atrNow)
		return TrendSignal{
			HasSignal:      true,
			Side:           "BUY",
			Confidence:     conf,
			Reason:         "M15 trend-following long: close>EMA20>EMA50 with positive slope",
			StopLossPips:   slPips,
			TakeProfitPips: tpPips,
		}, nil
	}

	// Sell trend regime: price below fast, fast below slow, and fast slope down.
	if lastClose < fastNow && fastNow < slowNow && fastNow < fastPrev && slowNow <= slowPrev {
		conf := confidence(lastClose, fastNow, slowNow, atrNow)
		slPips, tpPips := sltpPips(input.Symbol, atrNow)
		return TrendSignal{
			HasSignal:      true,
			Side:           "SELL",
			Confidence:     conf,
			Reason:         "M15 trend-following short: close<EMA20<EMA50 with negative slope",
			StopLossPips:   slPips,
			TakeProfitPips: tpPips,
		}, nil
	}

	return TrendSignal{
		HasSignal: false,
		Reason:    "no clear trend setup",
	}, nil
}

func ema(values []float64, period int) float64 {
	if len(values) == 0 {
		return 0
	}
	if period <= 1 {
		return values[len(values)-1]
	}
	k := 2.0 / float64(period+1)
	v := values[0]
	for i := 1; i < len(values); i++ {
		v = values[i]*k + v*(1-k)
	}
	return v
}

func atr(candles []Candle, length int) float64 {
	if len(candles) < 2 {
		return 0
	}
	if length <= 1 {
		length = 1
	}
	start := max(1, len(candles)-length)
	sum := 0.0
	count := 0
	for i := start; i < len(candles); i++ {
		curr := candles[i]
		prev := candles[i-1]
		tr := maxFloat(
			curr.High-curr.Low,
			math.Abs(curr.High-prev.Close),
			math.Abs(curr.Low-prev.Close),
		)
		sum += tr
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func confidence(close, fast, slow, atrValue float64) float64 {
	if close <= 0 || slow <= 0 {
		return 0.55
	}
	trendGap := math.Abs(fast-slow) / slow
	atrPct := atrValue / close

	score := 0.55
	score += minFloat(0.25, trendGap*12)
	score += minFloat(0.12, atrPct*3)
	score += minFloat(0.08, math.Abs(close-fast)/close*20)

	if score > 0.90 {
		score = 0.90
	}
	if score < 0.55 {
		score = 0.55
	}
	return score
}

func sltpPips(symbol string, atrValue float64) (float64, float64) {
	pip := pipSize(symbol)
	sl := (atrValue / pip) * 1.5
	if sl < 8 {
		sl = 8
	}
	tp := sl * 2.0
	return sl, tp
}

func pipSize(symbol string) float64 {
	upper := strings.ToUpper(symbol)
	if strings.Contains(upper, "JPY") {
		return 0.01
	}
	return 0.0001
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b, c float64) float64 {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}
