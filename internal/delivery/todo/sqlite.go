package todo

import (
	"database/sql"
	"fmt"
	"time"
)

// SQLiteStore 实现了基于 SQLite 的待办事项存储。
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore 从现有的数据库连接创建一个新的 SQLite 存储。
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}
	s := &SQLiteStore{
		db: db,
	}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

// init 自动创建数据表。
func (s *SQLiteStore) init() error {
	query := `
	CREATE TABLE IF NOT EXISTS todos (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT,
		completed BOOLEAN NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	`
	_, err := s.db.Exec(query)
	return err
}

// Create 创建一个新的待办事项。
func (s *SQLiteStore) Create(todo *Todo) error {
	if todo.CreatedAt.IsZero() {
		todo.CreatedAt = time.Now()
	}
	if todo.UpdatedAt.IsZero() {
		todo.UpdatedAt = time.Now()
	}

	query := `INSERT INTO todos (id, title, description, completed, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query,
		todo.ID,
		todo.Title,
		todo.Description,
		todo.Completed,
		todo.CreatedAt,
		todo.UpdatedAt,
	)
	return err
}

// Get 根据 ID 获取待办事项。
func (s *SQLiteStore) Get(id string) (*Todo, error) {
	query := `SELECT id, title, description, completed, created_at, updated_at FROM todos WHERE id = ?`
	row := s.db.QueryRow(query, id)

	var todo Todo
	err := row.Scan(&todo.ID, &todo.Title, &todo.Description, &todo.Completed, &todo.CreatedAt, &todo.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("todo not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	return &todo, nil
}

// List 获取所有待办事项。
func (s *SQLiteStore) List() ([]*Todo, error) {
	query := `SELECT id, title, description, completed, created_at, updated_at FROM todos ORDER BY created_at DESC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var todos []*Todo
	for rows.Next() {
		var todo Todo
		if err := rows.Scan(&todo.ID, &todo.Title, &todo.Description, &todo.Completed, &todo.CreatedAt, &todo.UpdatedAt); err != nil {
			return nil, err
		}
		todos = append(todos, &todo)
	}
	return todos, nil
}

// Update 更新现有的待办事项。
func (s *SQLiteStore) Update(todo *Todo) error {
	todo.UpdatedAt = time.Now()
	query := `UPDATE todos SET title = ?, description = ?, completed = ?, updated_at = ? WHERE id = ?`
	res, err := s.db.Exec(query, todo.Title, todo.Description, todo.Completed, todo.UpdatedAt, todo.ID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("todo not found: %s", todo.ID)
	}
	return nil
}

// Delete 删除待办事项。
func (s *SQLiteStore) Delete(id string) error {
	query := `DELETE FROM todos WHERE id = ?`
	res, err := s.db.Exec(query, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("todo not found: %s", id)
	}
	return nil
}

var _ Store = (*SQLiteStore)(nil)
