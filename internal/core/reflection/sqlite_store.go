package reflection

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// SQLiteStore 实现了基于 SQLite 的反思记录存储。
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore 创建并初始化反思存储。
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	s := &SQLiteStore{db: db}
	if err := s.init(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) init(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS reflections (
		id TEXT PRIMARY KEY,
		agent_name TEXT NOT NULL,
		task_id TEXT NOT NULL,
		success BOOLEAN NOT NULL,
		duration_ms INTEGER NOT NULL,
		cost REAL NOT NULL,
		error_message TEXT,
		analysis TEXT,
		suggestions TEXT,
		confidence_score REAL DEFAULT 0.5,
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_reflections_agent ON reflections(agent_name);
	`
	_, err := s.db.ExecContext(ctx, query)
	return err
}

// Save 实现 Store 接口。
func (s *SQLiteStore) Save(ctx context.Context, r *Reflection) error {
	suggestionsJSON, _ := json.Marshal(r.Suggestions)

	query := `INSERT INTO reflections (id, agent_name, task_id, success, duration_ms, cost, error_message, analysis, suggestions, confidence_score, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	
	_, err := s.db.ExecContext(ctx, query,
		r.ID, r.AgentName, r.TaskID, r.Success, r.Duration.Milliseconds(),
		r.Cost, r.ErrorMessage, r.Analysis, string(suggestionsJSON), r.ConfidenceScore, r.CreatedAt,
	)
	return err
}

// ListRecent 实现 Store 接口。
func (s *SQLiteStore) ListRecent(ctx context.Context, limit int) ([]Reflection, error) {
	query := `SELECT id, agent_name, task_id, success, duration_ms, cost, error_message, analysis, suggestions, confidence_score, created_at 
	FROM reflections ORDER BY created_at DESC LIMIT ?`
	
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Reflection
	for rows.Next() {
		var r Reflection
		var durMs int64
		var suggestionsJSON string
		
		err := rows.Scan(&r.ID, &r.AgentName, &r.TaskID, &r.Success, &durMs, &r.Cost, &r.ErrorMessage, &r.Analysis, &suggestionsJSON, &r.ConfidenceScore, &r.CreatedAt)
		if err != nil {
			return nil, err
		}
		
		r.Duration = time.Duration(durMs) * time.Millisecond
		json.Unmarshal([]byte(suggestionsJSON), &r.Suggestions)
		results = append(results, r)
	}
	return results, nil
}
