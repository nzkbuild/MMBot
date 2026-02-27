package risk

import "testing"

func TestDeriveSnapshotMetrics_WithDailyPNLField(t *testing.T) {
	snapshot := map[string]interface{}{
		"equity":    10000.0,
		"daily_pnl": -210.0,
		"positions": []interface{}{
			map[string]interface{}{"profit": 20.0},
			map[string]interface{}{"profit": -30.0},
		},
	}
	metrics := DeriveSnapshotMetrics(snapshot)
	if metrics.OpenPositions != 2 {
		t.Fatalf("expected 2 open positions, got %d", metrics.OpenPositions)
	}
	if metrics.NetPnL != -210.0 {
		t.Fatalf("expected net pnl -210, got %.2f", metrics.NetPnL)
	}
	if metrics.DailyLossPct < 2.09 || metrics.DailyLossPct > 2.11 {
		t.Fatalf("expected daily loss ~2.1%%, got %.4f", metrics.DailyLossPct)
	}
}

func TestDeriveSnapshotMetrics_ComputesFromRealizedAndPositions(t *testing.T) {
	snapshot := map[string]interface{}{
		"day_start_equity":   5000.0,
		"realized_pnl_today": -60.0,
		"positions": []interface{}{
			map[string]interface{}{"profit": -20.0, "swap": -1.0, "commission": -2.0},
			map[string]interface{}{"profit": 10.0},
		},
	}
	metrics := DeriveSnapshotMetrics(snapshot)
	// net = -60 + (-23 + 10) = -73
	if metrics.NetPnL != -73 {
		t.Fatalf("expected net pnl -73, got %.2f", metrics.NetPnL)
	}
	if metrics.DailyLossPct < 1.45 || metrics.DailyLossPct > 1.47 {
		t.Fatalf("expected daily loss ~1.46%%, got %.4f", metrics.DailyLossPct)
	}
}
