package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	s := &SQLiteStore{db: db}
	if err := s.init(context.Background()); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *SQLiteStore) init(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		mode TEXT NOT NULL,
		description TEXT NOT NULL,
		input_json TEXT,
		status TEXT NOT NULL,
		trace_id TEXT,
		current_stage TEXT,
		final_output TEXT,
		error_summary TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);

	CREATE TABLE IF NOT EXISTS task_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL,
		message_id TEXT,
		from_actor TEXT,
		to_actor TEXT,
		event_type TEXT NOT NULL,
		payload_json TEXT,
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_task_events_task_created_at ON task_events(task_id, created_at DESC);
	`

	_, err := s.db.ExecContext(ctx, query)
	return err
}

func (s *SQLiteStore) Create(ctx context.Context, t *Task) error {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = t.CreatedAt
	}

	inputJSON, _ := json.Marshal(t.Input)
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO tasks (id, source, mode, description, input_json, status, trace_id, current_stage, final_output, error_summary, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID,
		t.Source,
		t.Mode,
		t.Description,
		string(inputJSON),
		t.Status,
		t.TraceID,
		t.CurrentStage,
		t.FinalOutput,
		t.ErrorSummary,
		t.CreatedAt,
		t.UpdatedAt,
	)
	return err
}

func (s *SQLiteStore) Get(ctx context.Context, id string) (*Task, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, source, mode, description, input_json, status, trace_id, current_stage, final_output, error_summary, created_at, updated_at
		 FROM tasks WHERE id = ?`,
		id,
	)

	var t Task
	var inputJSON string
	err := row.Scan(
		&t.ID,
		&t.Source,
		&t.Mode,
		&t.Description,
		&inputJSON,
		&t.Status,
		&t.TraceID,
		&t.CurrentStage,
		&t.FinalOutput,
		&t.ErrorSummary,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	if inputJSON != "" {
		_ = json.Unmarshal([]byte(inputJSON), &t.Input)
	}

	return &t, nil
}

func (s *SQLiteStore) List(ctx context.Context, limit int) ([]*Task, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, source, mode, description, input_json, status, trace_id, current_stage, final_output, error_summary, created_at, updated_at
		 FROM tasks ORDER BY created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		var t Task
		var inputJSON string
		if err := rows.Scan(
			&t.ID,
			&t.Source,
			&t.Mode,
			&t.Description,
			&inputJSON,
			&t.Status,
			&t.TraceID,
			&t.CurrentStage,
			&t.FinalOutput,
			&t.ErrorSummary,
			&t.CreatedAt,
			&t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if inputJSON != "" {
			_ = json.Unmarshal([]byte(inputJSON), &t.Input)
		}
		tasks = append(tasks, &t)
	}

	return tasks, rows.Err()
}

func (s *SQLiteStore) Update(ctx context.Context, t *Task) error {
	t.UpdatedAt = time.Now()
	inputJSON, _ := json.Marshal(t.Input)
	res, err := s.db.ExecContext(
		ctx,
		`UPDATE tasks
		 SET source = ?, mode = ?, description = ?, input_json = ?, status = ?, trace_id = ?, current_stage = ?, final_output = ?, error_summary = ?, updated_at = ?
		 WHERE id = ?`,
		t.Source,
		t.Mode,
		t.Description,
		string(inputJSON),
		t.Status,
		t.TraceID,
		t.CurrentStage,
		t.FinalOutput,
		t.ErrorSummary,
		t.UpdatedAt,
		t.ID,
	)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("task not found: %s", t.ID)
	}
	return nil
}

func (s *SQLiteStore) AppendEvent(ctx context.Context, e *Event) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}

	payloadJSON, _ := json.Marshal(e.Payload)
	res, err := s.db.ExecContext(
		ctx,
		`INSERT INTO task_events (task_id, message_id, from_actor, to_actor, event_type, payload_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.TaskID,
		e.MessageID,
		e.From,
		e.To,
		e.Type,
		string(payloadJSON),
		e.CreatedAt,
	)
	if err != nil {
		return err
	}

	id, err := res.LastInsertId()
	if err == nil {
		e.ID = id
	}
	return nil
}

func (s *SQLiteStore) ListEvents(ctx context.Context, taskID string, limit int) ([]*Event, error) {
	if limit <= 0 {
		limit = 200
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, task_id, message_id, from_actor, to_actor, event_type, payload_json, created_at
		 FROM task_events WHERE task_id = ? ORDER BY created_at ASC LIMIT ?`,
		taskID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		var e Event
		var payloadJSON string
		if err := rows.Scan(
			&e.ID,
			&e.TaskID,
			&e.MessageID,
			&e.From,
			&e.To,
			&e.Type,
			&payloadJSON,
			&e.CreatedAt,
		); err != nil {
			return nil, err
		}
		if payloadJSON != "" {
			_ = json.Unmarshal([]byte(payloadJSON), &e.Payload)
		}
		events = append(events, &e)
	}

	return events, rows.Err()
}

var _ Store = (*SQLiteStore)(nil)
