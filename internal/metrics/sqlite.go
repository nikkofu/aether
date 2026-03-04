package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// UsageRecord 记录单次 LLM 调用的消耗情况。
type UsageRecord struct {
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	EstimatedCost    float64   `json:"estimated_cost"`
	CreatedAt        time.Time `json:"created_at"`
}

// Tracker 定义了指标追踪的通用接口。
type Tracker interface {
	// RecordUsage 持久化一条使用记录。
	RecordUsage(ctx context.Context, record UsageRecord) error
	// Close 释放资源。
	Close() error
}

// SQLiteTracker 实现了基于 SQLite 的指标持久化存储。
type SQLiteTracker struct {
	db *sql.DB
}

// NewSQLiteTracker 从现有的数据库连接创建一个新的 SQLiteTracker 实例并初始化表结构。
func NewSQLiteTracker(db *sql.DB) (*SQLiteTracker, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库连接不能为空")
	}
	t := &SQLiteTracker{db: db}
	if err := t.init(context.Background()); err != nil {
		return nil, err
	}
	return t, nil
}

// init 自动创建 usage 数据表。
func (t *SQLiteTracker) init(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS usage (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		prompt_tokens INTEGER DEFAULT 0,
		completion_tokens INTEGER DEFAULT 0,
		total_tokens INTEGER DEFAULT 0,
		estimated_cost REAL DEFAULT 0.0,
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_usage_created_at ON usage(created_at);
	`
	_, err := t.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("初始化 usage 表失败: %w", err)
	}
	return nil
}

// RecordUsage 将使用记录写入 SQLite。
func (t *SQLiteTracker) RecordUsage(ctx context.Context, r UsageRecord) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	if r.TotalTokens == 0 {
		r.TotalTokens = r.PromptTokens + r.CompletionTokens
	}

	query := `
	INSERT INTO usage (provider, model, prompt_tokens, completion_tokens, total_tokens, estimated_cost, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := t.db.ExecContext(ctx, query,
		r.Provider,
		r.Model,
		r.PromptTokens,
		r.CompletionTokens,
		r.TotalTokens,
		r.EstimatedCost,
		r.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("记录使用情况失败: %w", err)
	}
	return nil
}

// Close 不会真正关闭共享的数据库句柄，仅为实现接口。
func (t *SQLiteTracker) Close() error {
	return nil
}

// 确保 SQLiteTracker 实现了 Tracker 接口。
var _ Tracker = (*SQLiteTracker)(nil)
