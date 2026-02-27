package memory

import (
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"mmbot/internal/domain"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	mu sync.RWMutex

	tokenTTL time.Duration

	paused bool

	eaSessions map[string]domain.EASession

	commands     map[string]domain.Command
	commandOrder []string

	events []domain.Event

	openPositionsByAccount map[string]int
	dailyLossByAccount     map[string]float64
	lastSeenByDevice       map[string]time.Time

	openAIState       map[string]domain.OAuthState
	openAIConnection  *domain.ProviderConnection
	positionSnapshots map[string]map[string]interface{}
}

func NewStore(tokenTTL time.Duration) *Store {
	return &Store{
		tokenTTL:               tokenTTL,
		eaSessions:             make(map[string]domain.EASession),
		commands:               make(map[string]domain.Command),
		commandOrder:           make([]string, 0, 64),
		events:                 make([]domain.Event, 0, 256),
		openPositionsByAccount: make(map[string]int),
		dailyLossByAccount:     make(map[string]float64),
		lastSeenByDevice:       make(map[string]time.Time),
		openAIState:            make(map[string]domain.OAuthState),
		positionSnapshots:      make(map[string]map[string]interface{}),
	}
}

func (s *Store) IssueEASession(accountID, deviceID string) domain.EASession {
	s.mu.Lock()
	defer s.mu.Unlock()

	token := uuid.NewString()
	session := domain.EASession{
		Token:     token,
		AccountID: accountID,
		DeviceID:  deviceID,
		ExpiresAt: time.Now().UTC().Add(s.tokenTTL),
		Scopes:    []string{"trade:execute", "trade:read", "account:read"},
	}
	s.eaSessions[token] = session
	return session
}

func (s *Store) ValidateEASession(token string) (domain.EASession, error) {
	s.mu.RLock()
	session, ok := s.eaSessions[token]
	s.mu.RUnlock()
	if !ok {
		return domain.EASession{}, ErrNotFound
	}
	if session.ExpiresAt.Before(time.Now().UTC()) {
		return domain.EASession{}, errors.New("ea token expired")
	}
	return session, nil
}

func (s *Store) TouchDevice(deviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSeenByDevice[deviceID] = time.Now().UTC()
}

func (s *Store) SavePositionSnapshot(accountID string, snapshot map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.positionSnapshots[accountID] = snapshot
}

func (s *Store) EnqueueCommand(cmd domain.Command) domain.Command {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cmd.ID == "" {
		cmd.ID = uuid.NewString()
	}
	if cmd.CreatedAt.IsZero() {
		cmd.CreatedAt = time.Now().UTC()
	}
	if cmd.Status == "" {
		cmd.Status = domain.CommandStatusQueued
	}
	s.commands[cmd.ID] = cmd
	s.commandOrder = append(s.commandOrder, cmd.ID)
	return cmd
}

func (s *Store) NextQueuedCommand(accountID string) (domain.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range s.commandOrder {
		cmd := s.commands[id]
		if cmd.AccountID != accountID {
			continue
		}
		if cmd.Status != domain.CommandStatusQueued {
			continue
		}
		if time.Now().UTC().After(cmd.ExpiresAt) {
			cmd.Status = domain.CommandStatusFailed
			cmd.Reason = "expired before dispatch"
			s.commands[id] = cmd
			continue
		}
		cmd.Status = domain.CommandStatusDispatched
		s.commands[id] = cmd
		return cmd, nil
	}

	return domain.Command{}, ErrNotFound
}

func (s *Store) MarkCommandResult(result domain.CommandResult) (domain.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cmd, ok := s.commands[result.CommandID]
	if !ok {
		return domain.Command{}, ErrNotFound
	}
	if result.Status == "SUCCESS" {
		cmd.Status = domain.CommandStatusSuccess
	} else {
		cmd.Status = domain.CommandStatusFailed
	}
	s.commands[result.CommandID] = cmd
	return cmd, nil
}

func (s *Store) SetPaused(paused bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = paused
}

func (s *Store) IsPaused() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.paused
}

func (s *Store) AppendEvent(eventType domain.EventType, accountID string, payload map[string]interface{}) domain.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	event := domain.Event{
		ID:        uuid.NewString(),
		AccountID: accountID,
		Type:      eventType,
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	}
	s.events = append(s.events, event)
	return event
}

func (s *Store) ListEvents(limit int) []domain.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 20
	}
	if len(s.events) == 0 {
		return []domain.Event{}
	}
	start := max(len(s.events)-limit, 0)
	out := slices.Clone(s.events[start:])
	slices.Reverse(out)
	return out
}

func (s *Store) OpenPositions(accountID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.openPositionsByAccount[accountID]
}

func (s *Store) AdjustOpenPositions(accountID string, delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openPositionsByAccount[accountID] += delta
	if s.openPositionsByAccount[accountID] < 0 {
		s.openPositionsByAccount[accountID] = 0
	}
}

func (s *Store) DailyLoss(accountID string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dailyLossByAccount[accountID]
}

func (s *Store) SetDailyLoss(accountID string, lossPct float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dailyLossByAccount[accountID] = lossPct
}

func (s *Store) SaveOAuthState(state domain.OAuthState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openAIState[state.State] = state
}

func (s *Store) ConsumeOAuthState(state string) (domain.OAuthState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.openAIState[state]
	if !ok {
		return domain.OAuthState{}, ErrNotFound
	}
	delete(s.openAIState, state)
	return v, nil
}

func (s *Store) SaveOpenAIConnection(conn domain.ProviderConnection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openAIConnection = &conn
}

func (s *Store) GetOpenAIConnection() (domain.ProviderConnection, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.openAIConnection == nil {
		return domain.ProviderConnection{}, false
	}
	return *s.openAIConnection, true
}

func (s *Store) ClearOpenAIConnection() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openAIConnection = nil
}
