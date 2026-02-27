package store

import "mmbot/internal/domain"

// Store defines the runtime persistence contract used by the HTTP layer.
type Store interface {
	IssueEASession(accountID, deviceID string) domain.EASession
	ValidateEASession(token string) (domain.EASession, error)
	TouchDevice(deviceID string)
	SavePositionSnapshot(accountID string, snapshot map[string]interface{})

	EnqueueCommand(cmd domain.Command) domain.Command
	NextQueuedCommand(accountID string) (domain.Command, error)
	MarkCommandResult(result domain.CommandResult) (domain.Command, error)

	SetPaused(paused bool)
	IsPaused() bool

	AppendEvent(eventType domain.EventType, accountID string, payload map[string]interface{}) domain.Event
	ListEvents(limit int) []domain.Event

	OpenPositions(accountID string) int
	AdjustOpenPositions(accountID string, delta int)
	DailyLoss(accountID string) float64
	SetDailyLoss(accountID string, lossPct float64)

	SaveOAuthState(state domain.OAuthState)
	ConsumeOAuthState(state string) (domain.OAuthState, error)
	SaveOpenAIConnection(conn domain.ProviderConnection)
	GetOpenAIConnection() (domain.ProviderConnection, bool)
	ClearOpenAIConnection()
}
