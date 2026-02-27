package risk

import (
	"math"
	"strings"
)

type SnapshotMetrics struct {
	OpenPositions int     `json:"open_positions"`
	DailyLossPct  float64 `json:"daily_loss_pct"`
	Equity        float64 `json:"equity"`
	NetPnL        float64 `json:"net_pnl"`
}

func DeriveSnapshotMetrics(snapshot map[string]interface{}) SnapshotMetrics {
	openPositions := countPositions(snapshot)
	equity := firstFloat(snapshot,
		"day_start_equity",
		"equity",
		"account_equity",
		"metrics.equity",
		"account.equity",
		"balance",
		"account.balance",
	)

	netPnL := dailyPnL(snapshot)
	dailyLossPct := 0.0
	if equity > 0 && netPnL < 0 {
		dailyLossPct = math.Abs(netPnL) / equity * 100.0
	}
	if dailyLossPct < 0 {
		dailyLossPct = 0
	}
	return SnapshotMetrics{
		OpenPositions: openPositions,
		DailyLossPct:  dailyLossPct,
		Equity:        equity,
		NetPnL:        netPnL,
	}
}

func countPositions(snapshot map[string]interface{}) int {
	for _, key := range []string{"positions", "open_positions"} {
		if arr, ok := getArray(snapshot, key); ok {
			return len(arr)
		}
	}
	return int(firstFloat(snapshot, "open_positions_count", "metrics.open_positions_count"))
}

func dailyPnL(snapshot map[string]interface{}) float64 {
	// If explicit daily total exists, prefer it to avoid double-counting.
	if v, ok := getFloat(snapshot, "daily_pnl"); ok {
		return v
	}
	if v, ok := getFloat(snapshot, "metrics.daily_pnl"); ok {
		return v
	}

	realized := firstFloat(snapshot,
		"closed_pnl_today",
		"daily_realized_pnl",
		"realized_pnl_today",
		"metrics.realized_pnl_today",
	)

	unrealized := 0.0
	if positions, ok := getArray(snapshot, "positions"); ok {
		for _, item := range positions {
			pm, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			unrealized += valueOrZero(pm, "profit")
			unrealized += valueOrZero(pm, "swap")
			unrealized += valueOrZero(pm, "commission")
		}
	}
	return realized + unrealized
}

func valueOrZero(m map[string]interface{}, key string) float64 {
	v, _ := getFloat(m, key)
	return v
}

func firstFloat(snapshot map[string]interface{}, keys ...string) float64 {
	for _, key := range keys {
		if v, ok := getFloat(snapshot, key); ok {
			return v
		}
	}
	return 0
}

func getArray(snapshot map[string]interface{}, path string) ([]interface{}, bool) {
	v, ok := getByPath(snapshot, path)
	if !ok {
		return nil, false
	}
	arr, ok := v.([]interface{})
	return arr, ok
}

func getFloat(snapshot map[string]interface{}, path string) (float64, bool) {
	v, ok := getByPath(snapshot, path)
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint64:
		return float64(n), true
	case uint32:
		return float64(n), true
	case string:
		// Keep parser strict/simple for MVP: reject non-empty strings as unknown.
		if strings.TrimSpace(n) == "" {
			return 0, false
		}
		return 0, false
	default:
		return 0, false
	}
}

func getByPath(snapshot map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	var current interface{} = snapshot
	for _, p := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		next, ok := m[p]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}
