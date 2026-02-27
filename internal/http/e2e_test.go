package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mmbot/internal/config"
	"mmbot/internal/integrations/openclaw"
	"mmbot/internal/integrations/telegram"
	"mmbot/internal/service/risk"
	"mmbot/internal/store/memory"
)

func TestE2E_PaperFlow_WithOAuthAndEAExecution(t *testing.T) {
	oauthSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		grantType := r.Form.Get("grant_type")
		var resp map[string]interface{}
		if grantType == "authorization_code" {
			resp = map[string]interface{}{
				"access_token":  "access_abc",
				"refresh_token": "refresh_abc",
				"token_type":    "Bearer",
				"expires_in":    3600,
				"scope":         "models.read models.inference",
			}
		} else if grantType == "refresh_token" {
			resp = map[string]interface{}{
				"access_token":  "access_refreshed",
				"refresh_token": "refresh_abc",
				"token_type":    "Bearer",
				"expires_in":    3600,
				"scope":         "models.read models.inference",
			}
		} else {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer oauthSrv.Close()

	cfg := config.Config{
		AdminUsername:      "admin",
		AdminPassword:      "pw",
		JWTSecret:          "jwt-secret",
		EAConnectCode:      "MMBOT-ONE-TIME-CODE",
		EATokenTTL:         24 * time.Hour,
		AIMinConfidence:    0.70,
		MaxDailyLossPct:    2.0,
		MaxOpenPositions:   3,
		MaxSpreadPips:      2.0,
		OpenAIClientID:     "client-id",
		OpenAIClientSecret: "client-secret",
		OpenAIAuthURL:      oauthSrv.URL + "/oauth/authorize",
		OpenAITokenURL:     oauthSrv.URL + "/oauth/token",
		OpenAIScopes:       "models.read models.inference",
		OpenAIRedirectURI:  "http://localhost/oauth/callback",
		OpenAIRefreshSkew:  2 * time.Minute,
		OpenClawTimeout:    1 * time.Second,
	}

	store := memory.NewStore(24 * time.Hour)
	srv := NewServer(
		cfg,
		store,
		risk.NewEngine(cfg.MaxOpenPositions, cfg.MaxDailyLossPct, cfg.AIMinConfidence, cfg.MaxSpreadPips),
		telegram.NewNotifier("", ""),
		openclaw.NewClient("", time.Second, 0, 100*time.Millisecond, time.Second),
	)
	api := httptest.NewServer(srv.Router())
	defer api.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	// Admin login
	adminLoginResp := postJSON(t, client, api.URL+"/admin/login", map[string]string{
		"username": "admin",
		"password": "pw",
	}, "")
	adminToken := strField(t, adminLoginResp, "token")
	if adminToken == "" {
		t.Fatalf("expected admin token")
	}

	// OAuth start -> callback
	startResp := getJSON(t, client, api.URL+"/oauth/openai/start", "")
	state := strField(t, startResp, "state")
	if state == "" {
		t.Fatalf("expected oauth state")
	}
	_ = getJSON(t, client, fmt.Sprintf("%s/oauth/openai/callback?state=%s&code=%s", api.URL, state, "code123"), "")

	// EA register + heartbeat
	eaRegResp := postJSON(t, client, api.URL+"/ea/register", map[string]string{
		"connect_code": "MMBOT-ONE-TIME-CODE",
		"account_id":   "paper-1",
		"device_id":    "dev-1",
	}, "")
	eaToken := strField(t, eaRegResp, "token")
	if eaToken == "" {
		t.Fatalf("expected ea token")
	}
	_ = postJSON(t, client, api.URL+"/ea/heartbeat", map[string]interface{}{}, eaToken)

	// Strategy evaluate -> command queued
	strategyPayload := map[string]interface{}{
		"account_id":  "paper-1",
		"symbol":      "EURUSD",
		"spread_pips": 1.1,
		"candles":     uptrendCandles(120),
	}
	evalResp := postJSON(t, client, api.URL+"/admin/strategy/evaluate", strategyPayload, adminToken)
	allowed := boolField(evalResp, "allowed")
	if !allowed {
		t.Fatalf("expected allowed strategy response, got: %#v", evalResp)
	}

	// EA polls execute and reports success
	execResp := postJSON(t, client, api.URL+"/ea/execute", map[string]interface{}{}, eaToken)
	cmdType := strings.ToUpper(strField(t, execResp, "type"))
	if cmdType != "OPEN" {
		t.Fatalf("expected OPEN command, got %s", cmdType)
	}
	cmdID := strField(t, execResp, "command_id")
	if cmdID == "" {
		t.Fatalf("expected command_id")
	}
	_ = postJSON(t, client, api.URL+"/ea/result", map[string]interface{}{
		"command_id":    cmdID,
		"status":        "SUCCESS",
		"broker_ticket": "12345",
		"executed_at":   time.Now().UTC().Format(time.RFC3339),
	}, eaToken)

	// Summary should reflect open position = 1
	summary := getJSON(t, client, api.URL+"/dashboard/summary?account_id=paper-1", adminToken)
	openPositions, ok := numField(summary, "open_positions")
	if !ok {
		t.Fatalf("missing open_positions from summary")
	}
	if int(openPositions) != 1 {
		t.Fatalf("expected open_positions=1, got %v", openPositions)
	}
}

func TestE2E_SyncDailyLossCircuitBreaker(t *testing.T) {
	cfg := config.Config{
		AdminUsername:    "admin",
		AdminPassword:    "pw",
		JWTSecret:        "jwt-secret",
		EAConnectCode:    "MMBOT-ONE-TIME-CODE",
		EATokenTTL:       24 * time.Hour,
		MaxDailyLossPct:  2.0,
		MaxOpenPositions: 3,
		MaxSpreadPips:    2.0,
		OpenClawTimeout:  time.Second,
	}
	store := memory.NewStore(24 * time.Hour)
	srv := NewServer(
		cfg,
		store,
		risk.NewEngine(cfg.MaxOpenPositions, cfg.MaxDailyLossPct, 0.70, cfg.MaxSpreadPips),
		telegram.NewNotifier("", ""),
		openclaw.NewClient("", time.Second, 0, 100*time.Millisecond, time.Second),
	)
	api := httptest.NewServer(srv.Router())
	defer api.Close()
	client := &http.Client{Timeout: 5 * time.Second}

	eaRegResp := postJSON(t, client, api.URL+"/ea/register", map[string]string{
		"connect_code": "MMBOT-ONE-TIME-CODE",
		"account_id":   "paper-1",
		"device_id":    "dev-1",
	}, "")
	eaToken := strField(t, eaRegResp, "token")

	syncResp := postJSON(t, client, api.URL+"/ea/sync", map[string]interface{}{
		"equity":     10000.0,
		"daily_pnl":  -250.0,
		"positions":  []interface{}{},
		"account_id": "paper-1",
		"device_id":  "dev-1",
	}, eaToken)
	if !boolField(syncResp, "triggered_circuit_breaker") {
		t.Fatalf("expected circuit breaker trigger, got %#v", syncResp)
	}

	heartbeat := postJSON(t, client, api.URL+"/ea/heartbeat", map[string]interface{}{}, eaToken)
	if !boolField(heartbeat, "paused") {
		t.Fatalf("expected paused=true after circuit breaker")
	}
}

func uptrendCandles(n int) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, n)
	base := 1.0800
	for i := 0; i < n; i++ {
		// Stronger, cleaner trend profile to clear the production
		// confidence gate (AI_MIN_CONFIDENCE=0.70) in E2E validation.
		step := float64(i) * 0.00065
		close := base + step
		if i%9 == 0 {
			close -= 0.0002
		}
		open := close - 0.0002
		high := close + 0.0008
		low := close - 0.0010
		out = append(out, map[string]interface{}{
			"time":  time.Unix(int64(1700000000+i*900), 0).UTC().Format(time.RFC3339),
			"open":  open,
			"high":  high,
			"low":   low,
			"close": close,
		})
	}
	return out
}

func postJSON(t *testing.T, client *http.Client, url string, body interface{}, bearerToken string) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var data map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&data)
		t.Fatalf("non-2xx status=%d body=%#v", resp.StatusCode, data)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func getJSON(t *testing.T, client *http.Client, url string, bearerToken string) map[string]interface{} {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var data map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&data)
		t.Fatalf("non-2xx status=%d body=%#v", resp.StatusCode, data)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func strField(t *testing.T, m map[string]interface{}, key string) string {
	t.Helper()
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func boolField(m map[string]interface{}, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

func numField(m map[string]interface{}, key string) (float64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	n, ok := v.(float64)
	return n, ok
}
