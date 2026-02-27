package domain

import "time"

type CommandType string

const (
	CommandOpen   CommandType = "OPEN"
	CommandClose  CommandType = "CLOSE"
	CommandMoveSL CommandType = "MOVE_SL"
	CommandSetTP  CommandType = "SET_TP"
	CommandPause  CommandType = "PAUSE"
	CommandResume CommandType = "RESUME"
	CommandNoop   CommandType = "NOOP"
)

type CommandStatus string

const (
	CommandStatusQueued     CommandStatus = "QUEUED"
	CommandStatusDispatched CommandStatus = "DISPATCHED"
	CommandStatusSuccess    CommandStatus = "SUCCESS"
	CommandStatusFailed     CommandStatus = "FAILED"
)

type EventType string

const (
	EventSignalProposed EventType = "SignalProposed"
	EventTradeExecuted  EventType = "TradeExecuted"
	EventTradeModified  EventType = "TradeModified"
	EventRiskTriggered  EventType = "RiskTriggered"
	EventBotPaused      EventType = "BotPaused"
)

type Command struct {
	ID        string        `json:"command_id"`
	AccountID string        `json:"account_id"`
	DeviceID  string        `json:"device_id"`
	Type      CommandType   `json:"type"`
	Symbol    string        `json:"symbol,omitempty"`
	Side      string        `json:"side,omitempty"`
	Volume    float64       `json:"volume,omitempty"`
	SL        float64       `json:"sl,omitempty"`
	TP        float64       `json:"tp,omitempty"`
	Reason    string        `json:"reason,omitempty"`
	Status    CommandStatus `json:"status"`
	ExpiresAt time.Time     `json:"expires_at"`
	CreatedAt time.Time     `json:"created_at"`
}

type CommandResult struct {
	CommandID    string `json:"command_id"`
	Status       string `json:"status"`
	BrokerTicket string `json:"broker_ticket"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	ExecutedAt   string `json:"executed_at,omitempty"`
}

type Event struct {
	ID        string                 `json:"event_id"`
	UserID    string                 `json:"user_id,omitempty"`
	AccountID string                 `json:"account_id,omitempty"`
	Type      EventType              `json:"event_type"`
	Payload   map[string]interface{} `json:"payload"`
	CreatedAt time.Time              `json:"created_at"`
}

type EASession struct {
	Token     string    `json:"token"`
	AccountID string    `json:"account_id"`
	DeviceID  string    `json:"device_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Scopes    []string  `json:"scopes"`
}

type OAuthState struct {
	State     string
	Provider  string
	CreatedAt time.Time
}

type ProviderConnection struct {
	Provider     string    `json:"provider"`
	AccessToken  string    `json:"-"`
	RefreshToken string    `json:"-"`
	Scopes       []string  `json:"scopes"`
	ExpiresAt    time.Time `json:"expires_at"`
	ConnectedAt  time.Time `json:"connected_at"`
}

type SignalInput struct {
	AccountID      string  `json:"account_id"`
	Symbol         string  `json:"symbol"`
	Side           string  `json:"side"`
	Confidence     float64 `json:"confidence"`
	Reason         string  `json:"reason"`
	SpreadPips     float64 `json:"spread_pips"`
	StopLossPips   float64 `json:"stop_loss_pips"`
	TakeProfitPips float64 `json:"take_profit_pips"`
}

type StrategyState struct {
	Paused        bool
	OpenPositions int
	DailyLossPct  float64
}

type RiskDecision struct {
	Allowed    bool   `json:"allowed"`
	DenyReason string `json:"deny_reason,omitempty"`
}

