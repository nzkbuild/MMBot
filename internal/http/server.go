package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"mmbot/internal/config"
	"mmbot/internal/domain"
	"mmbot/internal/integrations/openclaw"
	"mmbot/internal/integrations/telegram"
	"mmbot/internal/service/oauth"
	"mmbot/internal/service/risk"
	"mmbot/internal/service/strategy"
	storepkg "mmbot/internal/store"
)

type contextKey string

const (
	contextKeyAdminSubject contextKey = "admin_subject"
	contextKeyEASession    contextKey = "ea_session"
)

type Server struct {
	cfg         config.Config
	store       storepkg.Store
	riskEngine  *risk.Engine
	notifier    *telegram.Notifier
	openClaw    *openclaw.Client
	openAIOAuth *oauth.OpenAIClient
	trendEngine *strategy.TrendEngine
}

func NewServer(
	cfg config.Config,
	store storepkg.Store,
	riskEngine *risk.Engine,
	notifier *telegram.Notifier,
	openClaw *openclaw.Client,
) *Server {
	return &Server{
		cfg:        cfg,
		store:      store,
		riskEngine: riskEngine,
		notifier:   notifier,
		openClaw:   openClaw,
		openAIOAuth: &oauth.OpenAIClient{
			ClientID:     cfg.OpenAIClientID,
			ClientSecret: cfg.OpenAIClientSecret,
			AuthURL:      cfg.OpenAIAuthURL,
			TokenURL:     cfg.OpenAITokenURL,
			RedirectURI:  cfg.OpenAIRedirectURI,
			Scopes:       parseScopes(cfg.OpenAIScopes),
		},
		trendEngine: strategy.NewTrendEngine(),
	}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)

	r.Get("/health", s.handleHealth)

	r.Post("/admin/login", s.handleAdminLogin)
	r.Post("/ea/register", s.handleEARegister)

	r.Get("/oauth/openai/start", s.handleOpenAIStart)
	r.Get("/oauth/openai/callback", s.handleOpenAICallback)

	r.Group(func(protected chi.Router) {
		protected.Use(s.requireAdmin)
		protected.Post("/admin/logout", s.handleAdminLogout)
		protected.Post("/bot/pause", s.handlePause)
		protected.Post("/bot/resume", s.handleResume)
		protected.Get("/dashboard/summary", s.handleDashboardSummary)
		protected.Get("/events", s.handleListEvents)
		protected.Get("/oauth/openai/status", s.handleOpenAIStatus)
		protected.Post("/oauth/openai/disconnect", s.handleOpenAIDisconnect)
		protected.Post("/admin/signals/evaluate", s.handleEvaluateSignal)
		protected.Post("/admin/strategy/evaluate", s.handleTrendEvaluate)
	})

	r.Group(func(ea chi.Router) {
		ea.Use(s.requireEA)
		ea.Post("/ea/heartbeat", s.handleEAHeartbeat)
		ea.Post("/ea/sync", s.handleEASync)
		ea.Post("/ea/execute", s.handleEAExecute)
		ea.Post("/ea/result", s.handleEAResult)
	})

	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Username != s.cfg.AdminUsername || req.Password != s.cfg.AdminPassword {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, expiresAt, err := s.signAdminToken(req.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create admin token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":      token,
		"expires_at": expiresAt.Format(time.RFC3339),
		"type":       "Bearer",
	})
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleEARegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConnectCode string `json:"connect_code"`
		AccountID   string `json:"account_id"`
		DeviceID    string `json:"device_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ConnectCode != s.cfg.EAConnectCode {
		writeError(w, http.StatusUnauthorized, "invalid connect code")
		return
	}
	if req.AccountID == "" || req.DeviceID == "" {
		writeError(w, http.StatusBadRequest, "account_id and device_id are required")
		return
	}

	session := s.store.IssueEASession(req.AccountID, req.DeviceID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":      session.Token,
		"expires_at": session.ExpiresAt.Format(time.RFC3339),
		"scopes":     session.Scopes,
	})
}

func (s *Server) handleEAHeartbeat(w http.ResponseWriter, r *http.Request) {
	session, err := sessionFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "missing ea session")
		return
	}
	s.store.TouchDevice(session.DeviceID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":          true,
		"server_time": time.Now().UTC().Format(time.RFC3339),
		"paused":      s.store.IsPaused(),
	})
}

func (s *Server) handleEASync(w http.ResponseWriter, r *http.Request) {
	session, err := sessionFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "missing ea session")
		return
	}
	var payload map[string]interface{}
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.store.SavePositionSnapshot(session.AccountID, payload)

	metrics := risk.DeriveSnapshotMetrics(payload)
	s.store.SetOpenPositions(session.AccountID, metrics.OpenPositions)
	s.store.SetDailyLoss(session.AccountID, metrics.DailyLossPct)

	triggeredCircuitBreaker := false
	if metrics.DailyLossPct >= s.cfg.MaxDailyLossPct && !s.store.IsPaused() {
		triggeredCircuitBreaker = true
		s.store.SetPaused(true)
		s.emitEvent(domain.EventRiskTriggered, session.AccountID, map[string]interface{}{
			"reason":         "daily_loss_limit_hit_sync",
			"daily_loss_pct": metrics.DailyLossPct,
			"threshold_pct":  s.cfg.MaxDailyLossPct,
			"net_pnl":        metrics.NetPnL,
			"equity":         metrics.Equity,
		})
		s.emitEvent(domain.EventBotPaused, session.AccountID, map[string]interface{}{
			"paused": true,
			"source": "risk_circuit_breaker",
		})
		_ = s.notifier.Notify(r.Context(), fmt.Sprintf(
			"Daily loss circuit breaker triggered: %.2f%% >= %.2f%%. Bot paused.",
			metrics.DailyLossPct,
			s.cfg.MaxDailyLossPct,
		))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":                        true,
		"open_positions":            metrics.OpenPositions,
		"daily_loss_pct":            metrics.DailyLossPct,
		"triggered_circuit_breaker": triggeredCircuitBreaker,
	})
}

func (s *Server) handleEAExecute(w http.ResponseWriter, r *http.Request) {
	session, err := sessionFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "missing ea session")
		return
	}
	cmd, err := s.store.NextQueuedCommand(session.AccountID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"command_id": uuid.NewString(),
			"type":       domain.CommandNoop,
			"expires_at": time.Now().UTC().Add(2 * time.Second).Format(time.RFC3339),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"command_id": cmd.ID,
		"type":       cmd.Type,
		"symbol":     cmd.Symbol,
		"side":       cmd.Side,
		"volume":     cmd.Volume,
		"sl":         cmd.SL,
		"tp":         cmd.TP,
		"reason":     cmd.Reason,
		"expires_at": cmd.ExpiresAt.Format(time.RFC3339),
	})
}

func (s *Server) handleEAResult(w http.ResponseWriter, r *http.Request) {
	session, err := sessionFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "missing ea session")
		return
	}
	var req domain.CommandResult
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cmd, err := s.store.MarkCommandResult(req)
	if err != nil {
		writeError(w, http.StatusNotFound, "command not found")
		return
	}
	success := strings.EqualFold(req.Status, "SUCCESS")
	if success {
		if cmd.Type == domain.CommandOpen {
			s.store.AdjustOpenPositions(session.AccountID, 1)
		}
		if cmd.Type == domain.CommandClose {
			s.store.AdjustOpenPositions(session.AccountID, -1)
		}
	}

	eventType := domain.EventTradeExecuted
	if cmd.Type == domain.CommandMoveSL || cmd.Type == domain.CommandSetTP {
		eventType = domain.EventTradeModified
	}
	event := s.emitEvent(eventType, session.AccountID, map[string]interface{}{
		"command_id":    req.CommandID,
		"status":        req.Status,
		"broker_ticket": req.BrokerTicket,
		"error_code":    req.ErrorCode,
		"error_message": req.ErrorMessage,
		"command_type":  cmd.Type,
		"symbol":        cmd.Symbol,
	})
	if success {
		_ = s.notifier.Notify(r.Context(), fmt.Sprintf("[%s] %s %s %.2f", cmd.Type, cmd.Side, cmd.Symbol, cmd.Volume))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"event_id": event.ID,
	})
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	s.store.SetPaused(true)
	event := s.emitEvent(domain.EventBotPaused, "", map[string]interface{}{
		"paused": true,
		"source": "admin",
	})
	_ = s.notifier.Notify(r.Context(), "MMBot paused: new OPEN commands are blocked.")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"event_id": event.ID,
	})
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	s.store.SetPaused(false)
	event := s.emitEvent(domain.EventBotPaused, "", map[string]interface{}{
		"paused": false,
		"source": "admin",
	})
	_ = s.notifier.Notify(r.Context(), "MMBot resumed: OPEN commands are allowed again.")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"event_id": event.ID,
	})
}

func (s *Server) handleEvaluateSignal(w http.ResponseWriter, r *http.Request) {
	var input domain.SignalInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.AccountID == "" {
		input.AccountID = "paper-1"
	}
	result := s.evaluateAndQueue(r.Context(), input)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleTrendEvaluate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID  string            `json:"account_id"`
		Symbol     string            `json:"symbol"`
		SpreadPips float64           `json:"spread_pips"`
		Candles    []strategy.Candle `json:"candles"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.AccountID == "" {
		req.AccountID = "paper-1"
	}

	sig, err := s.trendEngine.Evaluate(strategy.TrendInput{
		Symbol:     req.Symbol,
		Candles:    req.Candles,
		SpreadPips: req.SpreadPips,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !sig.HasSignal {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"has_signal": false,
			"reason":     sig.Reason,
		})
		return
	}

	input := domain.SignalInput{
		AccountID:      req.AccountID,
		Symbol:         req.Symbol,
		Side:           sig.Side,
		Confidence:     sig.Confidence,
		Reason:         sig.Reason,
		SpreadPips:     req.SpreadPips,
		StopLossPips:   sig.StopLossPips,
		TakeProfitPips: sig.TakeProfitPips,
	}
	result := s.evaluateAndQueue(r.Context(), input)
	result["has_signal"] = true
	result["strategy_signal"] = sig
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) evaluateAndQueue(ctx context.Context, input domain.SignalInput) map[string]interface{} {
	conn, connected := s.store.GetOpenAIConnection()
	conn, connected = s.ensureFreshOpenAIConnection(ctx, conn, connected)
	if !connected || conn.ExpiresAt.Before(time.Now().UTC()) {
		decision := domain.RiskDecision{Allowed: false, DenyReason: "provider_unavailable_fail_closed"}
		s.emitEvent(domain.EventRiskTriggered, input.AccountID, map[string]interface{}{
			"reason": decision.DenyReason,
			"input":  input,
		})
		return map[string]interface{}{
			"allowed":     false,
			"deny_reason": decision.DenyReason,
		}
	}

	state := domain.StrategyState{
		Paused:        s.store.IsPaused(),
		OpenPositions: s.store.OpenPositions(input.AccountID),
		DailyLossPct:  s.store.DailyLoss(input.AccountID),
	}
	decision := s.riskEngine.Evaluate(input, state)

	s.emitEvent(domain.EventSignalProposed, input.AccountID, map[string]interface{}{
		"symbol":     input.Symbol,
		"side":       input.Side,
		"confidence": input.Confidence,
		"allowed":    decision.Allowed,
		"reason":     input.Reason,
		"source":     "strategy",
	})

	if !decision.Allowed {
		s.emitEvent(domain.EventRiskTriggered, input.AccountID, map[string]interface{}{
			"reason": decision.DenyReason,
			"symbol": input.Symbol,
			"side":   input.Side,
		})
		_ = s.notifier.Notify(ctx, fmt.Sprintf("Risk trigger: %s (%s %s)", decision.DenyReason, input.Side, input.Symbol))
		return map[string]interface{}{
			"allowed":     false,
			"deny_reason": decision.DenyReason,
		}
	}

	cmd := s.store.EnqueueCommand(domain.Command{
		AccountID: input.AccountID,
		Type:      domain.CommandOpen,
		Symbol:    input.Symbol,
		Side:      strings.ToUpper(input.Side),
		Volume:    s.calculateVolume(),
		SL:        input.StopLossPips,
		TP:        input.TakeProfitPips,
		Reason:    input.Reason,
		ExpiresAt: time.Now().UTC().Add(30 * time.Second),
	})
	return map[string]interface{}{
		"allowed": decision.Allowed,
		"command": cmd,
	}
}

func (s *Server) handleDashboardSummary(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")
	if accountID == "" {
		accountID = "paper-1"
	}
	conn, connected := s.store.GetOpenAIConnection()
	conn, connected = s.ensureFreshOpenAIConnection(r.Context(), conn, connected)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"account_id":            accountID,
		"mode":                  "paper",
		"paused":                s.store.IsPaused(),
		"open_positions":        s.store.OpenPositions(accountID),
		"daily_loss_pct":        s.store.DailyLoss(accountID),
		"ai_provider_connected": connected && conn.ExpiresAt.After(time.Now().UTC()),
		"last_events":           s.store.ListEvents(20),
	})
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	limit := parseInt(r.URL.Query().Get("limit"), 20)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"events": s.store.ListEvents(limit),
		"count":  len(s.store.ListEvents(limit)),
	})
}

func (s *Server) handleOpenAIStart(w http.ResponseWriter, r *http.Request) {
	if s.cfg.OpenAIClientID == "" || s.cfg.OpenAIClientSecret == "" {
		writeError(w, http.StatusInternalServerError, "openai oauth is not configured")
		return
	}
	state := uuid.NewString()
	s.store.SaveOAuthState(domain.OAuthState{
		State:     state,
		Provider:  "openai",
		CreatedAt: time.Now().UTC(),
	})
	authURL, err := s.openAIOAuth.BuildAuthURL(state)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build openai auth url")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"provider": "openai",
		"state":    state,
		"auth_url": authURL,
	})
}

func (s *Server) handleOpenAICallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		writeError(w, http.StatusBadRequest, "state and code are required")
		return
	}
	if _, err := s.store.ConsumeOAuthState(state); err != nil {
		writeError(w, http.StatusBadRequest, "invalid oauth state")
		return
	}

	tokenResp, err := s.openAIOAuth.ExchangeCode(r.Context(), code)
	if err != nil {
		writeError(w, http.StatusBadGateway, "openai token exchange failed")
		return
	}
	scopeSource := tokenResp.Scope
	if strings.TrimSpace(scopeSource) == "" {
		scopeSource = s.cfg.OpenAIScopes
	}
	s.store.SaveOpenAIConnection(domain.ProviderConnection{
		Provider:     "openai",
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		Scopes:       parseScopes(scopeSource),
		ConnectedAt:  time.Now().UTC(),
		ExpiresAt:    time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"connected": true,
		"provider":  "openai",
	})
}

func (s *Server) handleOpenAIStatus(w http.ResponseWriter, r *http.Request) {
	conn, connected := s.store.GetOpenAIConnection()
	conn, connected = s.ensureFreshOpenAIConnection(r.Context(), conn, connected)
	if !connected {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"connected": false,
			"provider":  "openai",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"connected":    true,
		"provider":     conn.Provider,
		"expires_at":   conn.ExpiresAt.Format(time.RFC3339),
		"connected_at": conn.ConnectedAt.Format(time.RFC3339),
		"scopes":       conn.Scopes,
	})
}

func (s *Server) handleOpenAIDisconnect(w http.ResponseWriter, r *http.Request) {
	s.store.ClearOpenAIConnection()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) ensureFreshOpenAIConnection(ctx context.Context, conn domain.ProviderConnection, connected bool) (domain.ProviderConnection, bool) {
	if !connected {
		return conn, false
	}
	if conn.ExpiresAt.After(time.Now().UTC().Add(s.cfg.OpenAIRefreshSkew)) {
		return conn, true
	}
	if strings.TrimSpace(conn.RefreshToken) == "" {
		return conn, false
	}
	tokenResp, err := s.openAIOAuth.Refresh(ctx, conn.RefreshToken)
	if err != nil {
		return conn, false
	}
	newRefresh := tokenResp.RefreshToken
	if strings.TrimSpace(newRefresh) == "" {
		newRefresh = conn.RefreshToken
	}
	scopeSource := tokenResp.Scope
	if strings.TrimSpace(scopeSource) == "" {
		scopeSource = strings.Join(conn.Scopes, " ")
	}
	refreshed := domain.ProviderConnection{
		Provider:     "openai",
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: newRefresh,
		Scopes:       parseScopes(scopeSource),
		ConnectedAt:  conn.ConnectedAt,
		ExpiresAt:    time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}
	s.store.SaveOpenAIConnection(refreshed)
	return refreshed, true
}

func (s *Server) calculateVolume() float64 {
	// MVP fixed volume sizing for paper mode while risk % wiring is completed.
	return 0.01
}

func (s *Server) emitEvent(eventType domain.EventType, accountID string, payload map[string]interface{}) domain.Event {
	event := s.store.AppendEvent(eventType, accountID, payload)
	go func(evt domain.Event) {
		ctx, cancel := context.WithTimeout(context.Background(), s.cfg.OpenClawTimeout)
		defer cancel()
		_ = s.openClaw.Publish(ctx, evt)
	}(event)
	return event
}

func (s *Server) signAdminToken(subject string) (string, time.Time, error) {
	expiresAt := time.Now().UTC().Add(12 * time.Hour)
	claims := jwt.MapClaims{
		"sub": subject,
		"exp": expiresAt.Unix(),
		"iat": time.Now().UTC().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		parsed, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
			return []byte(s.cfg.JWTSecret), nil
		})
		if err != nil || !parsed.Valid {
			writeError(w, http.StatusUnauthorized, "invalid admin token")
			return
		}
		claims, ok := parsed.Claims.(jwt.MapClaims)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid admin claims")
			return
		}
		sub, _ := claims["sub"].(string)
		ctx := context.WithValue(r.Context(), contextKeyAdminSubject, sub)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) requireEA(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		session, err := s.store.ValidateEASession(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid ea token")
			return
		}
		ctx := context.WithValue(r.Context(), contextKeyEASession, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func sessionFromContext(ctx context.Context) (domain.EASession, error) {
	v := ctx.Value(contextKeyEASession)
	session, ok := v.(domain.EASession)
	if !ok {
		return domain.EASession{}, errors.New("ea session not found")
	}
	return session, nil
}

func bearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func parseInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func decodeJSON(r *http.Request, target interface{}) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseScopes(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	normalized := strings.ReplaceAll(raw, ",", " ")
	parts := strings.Fields(normalized)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
