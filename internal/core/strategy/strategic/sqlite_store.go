package strategic

import (
	"context"
	"database/sql"
)

// Store 定义了战略规划的持久化接口。
type Store interface {
	SaveVision(*Vision) error
	SaveGoals([]Goal) error
	SaveMilestones([]Milestone) error
	ListActiveGoals() ([]Goal, error)
	GetMilestones(goalID string) ([]Milestone, error)
	UpdateGoalStatus(id string, status string) error
	UpdateMilestoneStatus(id string, status string) error
}

// SQLiteStore 实现了战略规划数据的 SQLite 持久化。
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore 创建并初始化战略存储。
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	s := &SQLiteStore{db: db}
	if err := s.init(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) init(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS visions (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT,
			created_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS goals (
			id TEXT PRIMARY KEY,
			vision_id TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL,
			created_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS milestones (
			id TEXT PRIMARY KEY,
			goal_id TEXT NOT NULL,
			title TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at DATETIME NOT NULL
		);`,
	}

	for _, q := range queries {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) SaveVision(v *Vision) error {
	query := `INSERT INTO visions (id, title, description, created_at) VALUES (?, ?, ?, ?)`
	_, err := s.db.Exec(query, v.ID, v.Title, v.Description, v.CreatedAt)
	return err
}

func (s *SQLiteStore) SaveGoals(goals []Goal) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `INSERT INTO goals (id, vision_id, title, description, status, created_at) VALUES (?, ?, ?, ?, ?, ?)`
	for _, g := range goals {
		if _, err := tx.Exec(query, g.ID, g.VisionID, g.Title, g.Description, g.Status, g.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) SaveMilestones(milestones []Milestone) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `INSERT INTO milestones (id, goal_id, title, status, created_at) VALUES (?, ?, ?, ?, ?)`
	for _, m := range milestones {
		if _, err := tx.Exec(query, m.ID, m.GoalID, m.Title, m.Status, m.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListActiveGoals() ([]Goal, error) {
	query := `SELECT id, vision_id, title, description, status, created_at FROM goals WHERE status != 'completed'`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []Goal
	for rows.Next() {
		var g Goal
		if err := rows.Scan(&g.ID, &g.VisionID, &g.Title, &g.Description, &g.Status, &g.CreatedAt); err != nil {
			return nil, err
		}
		res = append(res, g)
	}
	return res, nil
}

func (s *SQLiteStore) GetMilestones(goalID string) ([]Milestone, error) {
	query := `SELECT id, goal_id, title, status, created_at FROM milestones WHERE goal_id = ? ORDER BY created_at ASC`
	rows, err := s.db.Query(query, goalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []Milestone
	for rows.Next() {
		var m Milestone
		if err := rows.Scan(&m.ID, &m.GoalID, &m.Title, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	return res, nil
}

func (s *SQLiteStore) UpdateGoalStatus(id string, status string) error {
	query := `UPDATE goals SET status = ? WHERE id = ?`
	_, err := s.db.Exec(query, status, id)
	return err
}

func (s *SQLiteStore) UpdateMilestoneStatus(id string, status string) error {
	query := `UPDATE milestones SET status = ? WHERE id = ?`
	_, err := s.db.Exec(query, status, id)
	return err
}

var _ Store = (*SQLiteStore)(nil)
