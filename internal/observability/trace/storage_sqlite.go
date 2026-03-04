package trace

import (
	"database/sql"
	"encoding/json"

	_ "modernc.org/sqlite"
)

type SQLiteTraceStorage struct {
	db *sql.DB
}

func NewSQLiteTraceStorage(path string) (*SQLiteTraceStorage, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	if err := createTables(db); err != nil {
		return nil, err
	}

	return &SQLiteTraceStorage{db: db}, nil
}

func createTables(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS traces (
			id TEXT PRIMARY KEY,
			org_id TEXT,
			started_at DATETIME,
			ended_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS spans (
			trace_id TEXT,
			span_id TEXT PRIMARY KEY,
			parent_span_id TEXT,
			org_id TEXT,
			agent_id TEXT,
			layer TEXT,
			action TEXT,
			status TEXT,
			started_at DATETIME,
			ended_at DATETIME,
			duration_ms INTEGER,
			metadata TEXT,
			FOREIGN KEY(trace_id) REFERENCES traces(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_traces_org_id ON traces(org_id)`,
		`CREATE INDEX IF NOT EXISTS idx_spans_trace_id ON spans(trace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_spans_org_id ON spans(org_id)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteTraceStorage) GetDB() *sql.DB {
	return s.db
}

func (s *SQLiteTraceStorage) InsertTrace(t *Trace) error {
	_, err := s.db.Exec(
		"INSERT INTO traces (id, org_id, started_at, ended_at) VALUES (?, ?, ?, ?)",
		t.ID, t.OrgID, t.StartedAt, t.EndedAt,
	)
	return err
}

func (s *SQLiteTraceStorage) InsertSpan(sp *Span) error {
	meta, _ := json.Marshal(sp.Metadata)
	_, err := s.db.Exec(
		`INSERT INTO spans (
			trace_id, span_id, parent_span_id, org_id, agent_id, layer, action, status, started_at, ended_at, duration_ms, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sp.TraceID, sp.SpanID, sp.ParentSpanID, sp.OrgID, sp.AgentID, sp.Layer, sp.Action, sp.Status, sp.StartedAt, sp.EndedAt, sp.DurationMs, string(meta),
	)
	return err
}

func (s *SQLiteTraceStorage) UpdateSpan(sp *Span) error {
	meta, _ := json.Marshal(sp.Metadata)
	_, err := s.db.Exec(
		"UPDATE spans SET status = ?, ended_at = ?, duration_ms = ?, metadata = ? WHERE span_id = ?",
		sp.Status, sp.EndedAt, sp.DurationMs, string(meta), sp.SpanID,
	)
	return err
}

func (s *SQLiteTraceStorage) GetTrace(traceID string) (*Trace, error) {
	var t Trace
	err := s.db.QueryRow("SELECT id, org_id, started_at, ended_at FROM traces WHERE id = ?", traceID).
		Scan(&t.ID, &t.OrgID, &t.StartedAt, &t.EndedAt)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query("SELECT trace_id, span_id, parent_span_id, org_id, agent_id, layer, action, status, started_at, ended_at, duration_ms, metadata FROM spans WHERE trace_id = ?", traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sp Span
		var meta string
		var endedAt sql.NullTime
		err := rows.Scan(
			&sp.TraceID, &sp.SpanID, &sp.ParentSpanID, &sp.OrgID, &sp.AgentID, &sp.Layer, &sp.Action, &sp.Status, &sp.StartedAt, &endedAt, &sp.DurationMs, &meta,
		)
		if err != nil {
			return nil, err
		}
		if endedAt.Valid {
			sp.EndedAt = endedAt.Time
		}
		json.Unmarshal([]byte(meta), &sp.Metadata)
		t.Spans = append(t.Spans, &sp)
	}

	return &t, nil
}

func (s *SQLiteTraceStorage) GetRecentTraces(orgID string, limit int) ([]*Trace, error) {
	rows, err := s.db.Query("SELECT id, org_id, started_at, ended_at FROM traces WHERE org_id = ? ORDER BY started_at DESC LIMIT ?", orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var traces []*Trace
	for rows.Next() {
		var t Trace
		if err := rows.Scan(&t.ID, &t.OrgID, &t.StartedAt, &t.EndedAt); err != nil {
			return nil, err
		}
		traces = append(traces, &t)
	}
	return traces, nil
}
