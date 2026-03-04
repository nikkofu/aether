package strategy

import (
	"context"
	"database/sql"
	"fmt"
)

// SQLiteStrategyStore 实现了基于 SQLite 的策略存储。
type SQLiteStrategyStore struct {
	db *sql.DB
}

func NewSQLiteStrategyStore(db *sql.DB) (*SQLiteStrategyStore, error) {
	s := &SQLiteStrategyStore{db: db}
	if err := s.init(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStrategyStore) init(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS strategies (
		agent_name TEXT PRIMARY KEY,
		prompt_hint TEXT,
		retry_limit INTEGER,
		routing_hint TEXT,
		updated_at DATETIME
	);`
	_, err := s.db.ExecContext(ctx, query)
	return err
}

func (s *SQLiteStrategyStore) Get(agentName string) (*Strategy, error) {
	query := `SELECT agent_name, prompt_hint, retry_limit, routing_hint, updated_at FROM strategies WHERE agent_name = ?`
	row := s.db.QueryRow(query, agentName)
	
	var st Strategy
	err := row.Scan(&st.AgentName, &st.PromptHint, &st.RetryLimit, &st.RoutingHint, &st.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("strategy not found")
	}
	return &st, err
}

func (s *SQLiteStrategyStore) Save(st *Strategy) error {
	query := `INSERT INTO strategies (agent_name, prompt_hint, retry_limit, routing_hint, updated_at)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(agent_name) DO UPDATE SET
		prompt_hint=excluded.prompt_hint,
		retry_limit=excluded.retry_limit,
		routing_hint=excluded.routing_hint,
		updated_at=excluded.updated_at`
	
	_, err := s.db.Exec(query, st.AgentName, st.PromptHint, st.RetryLimit, st.RoutingHint, st.UpdatedAt)
	return err
}
