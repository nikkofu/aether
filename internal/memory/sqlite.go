package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	_ "modernc.org/sqlite"
)

// SQLiteStore 是 Store 接口的持久化实现。
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore 创建一个新的 SQLite 存储并初始化。
func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("无法打开 SQLite 数据库: %w", err)
	}
	db.SetMaxOpenConns(1)

	return NewSQLiteStoreWithDB(db)
}

// NewSQLiteStoreWithDB 使用现有的数据库连接创建存储，方便资源复用。
func NewSQLiteStoreWithDB(db *sql.DB) (*SQLiteStore, error) {
	store := &SQLiteStore{db: db}
	if err := store.init(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

// init 执行数据库表结构初始化。
func (s *SQLiteStore) init(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS executions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		pipeline_id TEXT NOT NULL,
		node_id TEXT NOT NULL,
		output TEXT,
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_pipeline_id ON executions(pipeline_id);
	`
	_, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("初始化数据库表失败: %w", err)
	}
	return nil
}

// Save 实现 Store 接口。
func (s *SQLiteStore) Save(ctx context.Context, record ExecutionRecord) error {
	outputJSON, err := json.Marshal(record.Output)
	if err != nil {
		return fmt.Errorf("序列化输出数据失败: %w", err)
	}

	query := `INSERT INTO executions (pipeline_id, node_id, output, created_at) VALUES (?, ?, ?, ?)`
	_, err = s.db.ExecContext(ctx, query, record.PipelineID, record.NodeID, string(outputJSON), record.Timestamp)
	return err
}

// GetByPipeline 实现 Store 接口。
func (s *SQLiteStore) GetByPipeline(ctx context.Context, pipelineID string) ([]ExecutionRecord, error) {
	query := `SELECT pipeline_id, node_id, output, created_at FROM executions WHERE pipeline_id = ? ORDER BY created_at ASC`
	rows, err := s.db.QueryContext(ctx, query, pipelineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ExecutionRecord
	for rows.Next() {
		var r ExecutionRecord
		var outputStr string
		if err := rows.Scan(&r.PipelineID, &r.NodeID, &outputStr, &r.Timestamp); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(outputStr), &r.Output)
		records = append(records, r)
	}
	return records, nil
}

// ListRecent 实现 Store 接口。
func (s *SQLiteStore) ListRecent(ctx context.Context, limit int) ([]ExecutionRecord, error) {
	query := `SELECT pipeline_id, node_id, output, created_at FROM executions ORDER BY created_at DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ExecutionRecord
	for rows.Next() {
		var r ExecutionRecord
		var outputStr string
		if err := rows.Scan(&r.PipelineID, &r.NodeID, &outputStr, &r.Timestamp); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(outputStr), &r.Output)
		records = append(records, r)
	}
	return records, nil
}

// Close 实现 Store 接口。
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
