package issue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nikkofu/aether/pkg/logging"
)

// SQLiteHandler 实现了基于 SQLite 的问题上报处理器。
type SQLiteHandler struct {
	db     *sql.DB
	logger logging.Logger
}

// NewSQLiteHandler 从现有的数据库连接创建一个新的 SQLite 问题处理器。
func NewSQLiteHandler(db *sql.DB, l logging.Logger) (*SQLiteHandler, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}
	h := &SQLiteHandler{
		db:     db,
		logger: l,
	}
	if err := h.init(context.Background()); err != nil {
		return nil, err
	}
	return h, nil
}

// init 自动创建数据表和索引。
func (h *SQLiteHandler) init(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS issues (
		id TEXT PRIMARY KEY,
		severity TEXT NOT NULL,
		source TEXT NOT NULL,
		message TEXT NOT NULL,
		metadata TEXT,
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_issues_created_at ON issues(created_at);
	`
	_, err := h.db.ExecContext(ctx, query)
	return err
}

// Report 将 Issue 持久化到数据库，并通过结构化日志输出。
func (h *SQLiteHandler) Report(ctx context.Context, iss Issue) error {
	if iss.CreatedAt.IsZero() {
		iss.CreatedAt = time.Now()
	}

	metaBytes, _ := json.Marshal(iss.Metadata)

	// 1. 持久化到 SQLite
	query := `INSERT INTO issues (id, severity, source, message, metadata, created_at) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := h.db.ExecContext(ctx, query,
		iss.ID,
		iss.Severity,
		iss.Source,
		iss.Message,
		string(metaBytes),
		iss.CreatedAt,
	)

	// 2. 结构化日志输出
	if h.logger != nil {
		h.logger.Warn(ctx, "Issue Reported",
			logging.String("issue_id", iss.ID),
			logging.String("severity", iss.Severity),
			logging.String("source", iss.Source),
			logging.String("message", iss.Message),
		)
	}

	if err != nil {
		return fmt.Errorf("failed to save issue: %w", err)
	}
	return nil
}

// Close 此处理器不主动关闭数据库，由外部统一管理。
func (h *SQLiteHandler) Close() error {
	return nil
}

var _ Handler = (*SQLiteHandler)(nil)
