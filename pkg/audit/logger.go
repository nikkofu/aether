package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	_ "modernc.org/sqlite"
)

type EventType string

const (
	EventProposalCreated  EventType = "PROPOSAL_CREATED"
	EventProposalVoted    EventType = "PROPOSAL_VOTED"
	EventProposalPassed   EventType = "PROPOSAL_PASSED"
	EventConstRejected    EventType = "CONSTITUTION_REJECTED"
	EventEconomyCharge    EventType = "ECONOMY_CHARGE"
	EventReputationChange EventType = "REPUTATION_CHANGE"
)

type LogEntry struct {
	ID          int            `json:"id"`
	OrgID       string         `json:"org_id"`
	EventType   EventType      `json:"event_type"`
	Description string         `json:"description"`
	Metadata    map[string]any `json:"metadata"`
	CreatedAt   time.Time      `json:"created_at"`
}

type Logger interface {
	Log(ctx context.Context, orgID string, eventType EventType, desc string, meta map[string]any) error
	QueryByTimeRange(ctx context.Context, orgID string, start, end time.Time) ([]LogEntry, error)
}

type SQLiteLogger struct {
	db *sql.DB
}

func NewSQLiteLogger(db *sql.DB) (*SQLiteLogger, error) {
	l := &SQLiteLogger{db: db}
	if err := l.init(context.Background()); err != nil { return nil, err }
	return l, nil
}

func (l *SQLiteLogger) init(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		description TEXT NOT NULL,
		metadata TEXT,
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_audit_org_ts ON audit_logs(org_id, created_at);`
	_, err := l.db.ExecContext(ctx, query)
	return err
}

func (l *SQLiteLogger) Log(ctx context.Context, orgID string, eventType EventType, desc string, meta map[string]any) error {
	metaJSON, _ := json.Marshal(meta)
	query := `INSERT INTO audit_logs (org_id, event_type, description, metadata, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := l.db.ExecContext(ctx, query, orgID, eventType, desc, string(metaJSON), time.Now())
	return err
}

func (l *SQLiteLogger) QueryByTimeRange(ctx context.Context, orgID string, start, end time.Time) ([]LogEntry, error) {
	query := `SELECT id, org_id, event_type, description, metadata, created_at FROM audit_logs WHERE org_id = ? AND created_at BETWEEN ? AND ? ORDER BY created_at ASC`
	rows, err := l.db.QueryContext(ctx, query, orgID, start, end)
	if err != nil { return nil, err }
	defer rows.Close()
	var results []LogEntry
	for rows.Next() {
		var e LogEntry
		var metaStr string
		rows.Scan(&e.ID, &e.OrgID, &e.EventType, &e.Description, &metaStr, &e.CreatedAt)
		json.Unmarshal([]byte(metaStr), &e.Metadata)
		results = append(results, e)
	}
	return results, nil
}

var _ Logger = (*SQLiteLogger)(nil)
