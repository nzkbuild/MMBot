package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	_ "github.com/lib/pq"

	"mmbot/internal/domain"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db       *sql.DB
	tokenTTL time.Duration

	mu          sync.Mutex
	openAIState map[string]domain.OAuthState
}

func NewStore(databaseURL string, tokenTTL time.Duration) (*Store, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{
		db:          db,
		tokenTTL:    tokenTTL,
		openAIState: make(map[string]domain.OAuthState),
	}, nil
}

func (s *Store) IssueEASession(accountID, deviceID string) domain.EASession {
	now := time.Now().UTC()
	token := uuid.NewString()
	expiresAt := now.Add(s.tokenTTL)
	tokenHash := hashToken(token)

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err == nil {
		_, _ = tx.Exec(
			`insert into broker_accounts(id, broker_name, mode) values ($1, 'mt5', 'paper')
			 on conflict (id) do nothing`,
			accountID,
		)
		_, _ = tx.Exec(
			`insert into daily_risk_state(account_id, daily_loss_pct, open_positions, paused, updated_at)
			 values ($1, 0, 0, false, now()) on conflict (account_id) do nothing`,
			accountID,
		)
		_, _ = tx.Exec(
			`insert into ea_devices(id, account_id, token_hash, token_expires_at, last_seen_at)
			 values ($1, $2, $3, $4, $5)
			 on conflict (id) do update
			 set account_id = excluded.account_id,
			     token_hash = excluded.token_hash,
			     token_expires_at = excluded.token_expires_at,
			     last_seen_at = excluded.last_seen_at`,
			deviceID, accountID, tokenHash, expiresAt, now,
		)
		_ = tx.Commit()
	}

	return domain.EASession{
		Token:     token,
		AccountID: accountID,
		DeviceID:  deviceID,
		ExpiresAt: expiresAt,
		Scopes:    []string{"trade:execute", "trade:read", "account:read"},
	}
}

func (s *Store) ValidateEASession(token string) (domain.EASession, error) {
	tokenHash := hashToken(token)
	var accountID, deviceID string
	var expiresAt time.Time
	err := s.db.QueryRow(
		`select account_id, id, token_expires_at
		 from ea_devices
		 where token_hash = $1
		 limit 1`,
		tokenHash,
	).Scan(&accountID, &deviceID, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.EASession{}, ErrNotFound
		}
		return domain.EASession{}, err
	}
	if expiresAt.Before(time.Now().UTC()) {
		return domain.EASession{}, errors.New("ea token expired")
	}
	return domain.EASession{
		Token:     token,
		AccountID: accountID,
		DeviceID:  deviceID,
		ExpiresAt: expiresAt,
		Scopes:    []string{"trade:execute", "trade:read", "account:read"},
	}, nil
}

func (s *Store) TouchDevice(deviceID string) {
	_, _ = s.db.Exec(`update ea_devices set last_seen_at = now() where id = $1`, deviceID)
}

func (s *Store) SavePositionSnapshot(accountID string, snapshot map[string]interface{}) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return
	}
	_, _ = s.db.Exec(
		`insert into position_snapshots(account_id, snapshot, updated_at)
		 values ($1, $2::jsonb, now())
		 on conflict (account_id) do update
		 set snapshot = excluded.snapshot,
		     updated_at = now()`,
		accountID, string(raw),
	)
}

func (s *Store) EnqueueCommand(cmd domain.Command) domain.Command {
	if cmd.ID == "" {
		cmd.ID = uuid.NewString()
	}
	if cmd.CreatedAt.IsZero() {
		cmd.CreatedAt = time.Now().UTC()
	}
	if cmd.Status == "" {
		cmd.Status = domain.CommandStatusQueued
	}
	_, _ = s.db.Exec(
		`insert into commands(
			id, account_id, type, symbol, side, volume, sl, tp, reason, status, expires_at, created_at, updated_at
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now())`,
		cmd.ID,
		cmd.AccountID,
		string(cmd.Type),
		cmd.Symbol,
		cmd.Side,
		cmd.Volume,
		cmd.SL,
		cmd.TP,
		cmd.Reason,
		string(cmd.Status),
		cmd.ExpiresAt,
		cmd.CreatedAt,
	)
	return cmd
}

func (s *Store) NextQueuedCommand(accountID string) (domain.Command, error) {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return domain.Command{}, err
	}
	defer func() { _ = tx.Rollback() }()

	_, _ = tx.Exec(
		`update commands
		 set status = 'FAILED', reason = 'expired before dispatch', updated_at = now()
		 where account_id = $1 and status = 'QUEUED' and expires_at < now()`,
		accountID,
	)

	var cmd domain.Command
	var cmdType, status string
	err = tx.QueryRow(
		`select id, account_id, type, symbol, side, volume, sl, tp, reason, status, expires_at, created_at
		 from commands
		 where account_id = $1 and status = 'QUEUED' and expires_at >= now()
		 order by created_at asc
		 limit 1
		 for update skip locked`,
		accountID,
	).Scan(
		&cmd.ID,
		&cmd.AccountID,
		&cmdType,
		&cmd.Symbol,
		&cmd.Side,
		&cmd.Volume,
		&cmd.SL,
		&cmd.TP,
		&cmd.Reason,
		&status,
		&cmd.ExpiresAt,
		&cmd.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Command{}, ErrNotFound
		}
		return domain.Command{}, err
	}

	if _, err := tx.Exec(`update commands set status = 'DISPATCHED', updated_at = now() where id = $1`, cmd.ID); err != nil {
		return domain.Command{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Command{}, err
	}
	cmd.Type = domain.CommandType(cmdType)
	cmd.Status = domain.CommandStatusDispatched
	return cmd, nil
}

func (s *Store) MarkCommandResult(result domain.CommandResult) (domain.Command, error) {
	newStatus := domain.CommandStatusFailed
	if result.Status == "SUCCESS" {
		newStatus = domain.CommandStatusSuccess
	}
	res, err := s.db.Exec(
		`update commands set status = $2, updated_at = now() where id = $1`,
		result.CommandID, string(newStatus),
	)
	if err != nil {
		return domain.Command{}, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return domain.Command{}, ErrNotFound
	}

	var cmd domain.Command
	var cmdType, status string
	err = s.db.QueryRow(
		`select id, account_id, type, symbol, side, volume, sl, tp, reason, status, expires_at, created_at
		 from commands where id = $1`,
		result.CommandID,
	).Scan(
		&cmd.ID,
		&cmd.AccountID,
		&cmdType,
		&cmd.Symbol,
		&cmd.Side,
		&cmd.Volume,
		&cmd.SL,
		&cmd.TP,
		&cmd.Reason,
		&status,
		&cmd.ExpiresAt,
		&cmd.CreatedAt,
	)
	if err != nil {
		return domain.Command{}, err
	}
	cmd.Type = domain.CommandType(cmdType)
	cmd.Status = domain.CommandStatus(status)
	return cmd, nil
}

func (s *Store) SetPaused(paused bool) {
	raw, _ := json.Marshal(map[string]bool{"paused": paused})
	_, _ = s.db.Exec(
		`insert into app_state(key, value_json, updated_at)
		 values ('global_paused', $1::jsonb, now())
		 on conflict (key) do update
		 set value_json = excluded.value_json, updated_at = now()`,
		string(raw),
	)
}

func (s *Store) IsPaused() bool {
	var raw []byte
	err := s.db.QueryRow(`select value_json from app_state where key = 'global_paused'`).Scan(&raw)
	if err != nil {
		return false
	}
	var payload map[string]bool
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	return payload["paused"]
}

func (s *Store) AppendEvent(eventType domain.EventType, accountID string, payload map[string]interface{}) domain.Event {
	event := domain.Event{
		ID:        uuid.NewString(),
		AccountID: accountID,
		Type:      eventType,
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	}
	raw, _ := json.Marshal(payload)
	_, _ = s.db.Exec(
		`insert into events(id, account_id, event_type, payload, created_at)
		 values ($1, $2, $3, $4::jsonb, $5)`,
		event.ID, accountID, string(eventType), string(raw), event.CreatedAt,
	)
	return event
}

func (s *Store) ListEvents(limit int) []domain.Event {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(
		`select id, account_id, event_type, payload, created_at
		 from events order by created_at desc limit $1`,
		limit,
	)
	if err != nil {
		return []domain.Event{}
	}
	defer rows.Close()

	out := make([]domain.Event, 0, limit)
	for rows.Next() {
		var e domain.Event
		var eventType string
		var payloadRaw []byte
		if err := rows.Scan(&e.ID, &e.AccountID, &eventType, &payloadRaw, &e.CreatedAt); err != nil {
			continue
		}
		e.Type = domain.EventType(eventType)
		_ = json.Unmarshal(payloadRaw, &e.Payload)
		if e.Payload == nil {
			e.Payload = map[string]interface{}{}
		}
		out = append(out, e)
	}
	return out
}

func (s *Store) OpenPositions(accountID string) int {
	var n int
	err := s.db.QueryRow(`select open_positions from daily_risk_state where account_id = $1`, accountID).Scan(&n)
	if err != nil {
		return 0
	}
	return n
}

func (s *Store) AdjustOpenPositions(accountID string, delta int) {
	_, _ = s.db.Exec(
		`insert into daily_risk_state(account_id, daily_loss_pct, open_positions, paused, updated_at)
		 values ($1, 0, greatest($2, 0), false, now())
		 on conflict (account_id) do update
		 set open_positions = greatest(daily_risk_state.open_positions + $2, 0),
		     updated_at = now()`,
		accountID, delta,
	)
}

func (s *Store) DailyLoss(accountID string) float64 {
	var v float64
	err := s.db.QueryRow(`select daily_loss_pct from daily_risk_state where account_id = $1`, accountID).Scan(&v)
	if err != nil {
		return 0
	}
	return v
}

func (s *Store) SetDailyLoss(accountID string, lossPct float64) {
	_, _ = s.db.Exec(
		`insert into daily_risk_state(account_id, daily_loss_pct, open_positions, paused, updated_at)
		 values ($1, $2, 0, false, now())
		 on conflict (account_id) do update
		 set daily_loss_pct = excluded.daily_loss_pct,
		     updated_at = now()`,
		accountID, lossPct,
	)
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
	_, _ = s.db.Exec(
		`insert into oauth_provider_connections(provider, access_token_enc, refresh_token_enc, scopes, expires_at, connected_at)
		 values ($1, $2, $3, $4, $5, $6)
		 on conflict (provider) do update
		 set access_token_enc = excluded.access_token_enc,
		     refresh_token_enc = excluded.refresh_token_enc,
		     scopes = excluded.scopes,
		     expires_at = excluded.expires_at,
		     connected_at = excluded.connected_at`,
		conn.Provider,
		conn.AccessToken,
		conn.RefreshToken,
		pq.Array(conn.Scopes),
		conn.ExpiresAt,
		conn.ConnectedAt,
	)
}

func (s *Store) GetOpenAIConnection() (domain.ProviderConnection, bool) {
	var conn domain.ProviderConnection
	var scopes []string
	err := s.db.QueryRow(
		`select provider, access_token_enc, refresh_token_enc, scopes, expires_at, connected_at
		 from oauth_provider_connections
		 where provider = 'openai'
		 limit 1`,
	).Scan(
		&conn.Provider,
		&conn.AccessToken,
		&conn.RefreshToken,
		pq.Array(&scopes),
		&conn.ExpiresAt,
		&conn.ConnectedAt,
	)
	if err != nil {
		return domain.ProviderConnection{}, false
	}
	conn.Scopes = scopes
	return conn, true
}

func (s *Store) ClearOpenAIConnection() {
	_, _ = s.db.Exec(`delete from oauth_provider_connections where provider = 'openai'`)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
