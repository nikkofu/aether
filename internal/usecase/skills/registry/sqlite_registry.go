package registry

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nikkofu/aether/internal/usecase/skills"
	_ "modernc.org/sqlite"
)

// SQLiteSkillEngine 实现了 skills.SkillEngine 接口，支持版本树管理。
type SQLiteSkillEngine struct {
	db *sql.DB
}

func NewSQLiteSkillEngine(db *sql.DB) (*SQLiteSkillEngine, error) {
	if db == nil { return nil, fmt.Errorf("db required") }
	e := &SQLiteSkillEngine{db: db}
	if err := e.init(context.Background()); err != nil { return nil, err }
	return e, nil
}

func (e *SQLiteSkillEngine) init(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS skills (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			created_by TEXT,
			active BOOLEAN DEFAULT 1,
			created_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS skill_versions (
			skill_id TEXT NOT NULL,
			version TEXT NOT NULL,
			parent TEXT,
			code_path TEXT NOT NULL,
			entry_point TEXT,
			score REAL DEFAULT 0.0,
			active BOOLEAN DEFAULT 0,
			created_at DATETIME NOT NULL,
			PRIMARY KEY (skill_id, version),
			FOREIGN KEY (skill_id) REFERENCES skills(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_skill_versions_active ON skill_versions(skill_id, active);`,
	}
	for _, q := range queries {
		if _, err := e.db.ExecContext(ctx, q); err != nil { return err }
	}
	return nil
}

func (e *SQLiteSkillEngine) Register(ctx context.Context, s skills.Skill) error {
	if s.CreatedAt.IsZero() { s.CreatedAt = time.Now() }
	query := `INSERT INTO skills (id, name, description, created_by, active, created_at)
	VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET name=excluded.name, description=excluded.description, active=excluded.active`
	_, err := e.db.ExecContext(ctx, query, s.ID, s.Name, s.Description, s.CreatedBy, s.Active, s.CreatedAt)
	return err
}

func (e *SQLiteSkillEngine) RegisterVersion(ctx context.Context, v skills.SkillVersion) error {
	if v.CreatedAt.IsZero() { v.CreatedAt = time.Now() }
	query := `INSERT INTO skill_versions (skill_id, version, parent, code_path, entry_point, score, active, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := e.db.ExecContext(ctx, query, v.SkillID, v.Version, v.Parent, v.CodePath, v.EntryPoint, v.Score, v.Active, v.CreatedAt)
	return err
}

func (e *SQLiteSkillEngine) ActivateVersion(ctx context.Context, skillID, version string) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()

	// 1. 禁用该技能的所有其他版本
	_, err = tx.ExecContext(ctx, "UPDATE skill_versions SET active = 0 WHERE skill_id = ?", skillID)
	if err != nil { return err }

	// 2. 激活目标版本
	_, err = tx.ExecContext(ctx, "UPDATE skill_versions SET active = 1 WHERE skill_id = ? AND version = ?", skillID, version)
	if err != nil { return err }

	return tx.Commit()
}

func (e *SQLiteSkillEngine) ListActive(ctx context.Context) ([]skills.Skill, error) {
	rows, err := e.db.QueryContext(ctx, "SELECT id, name, description, created_by, active, created_at FROM skills WHERE active = 1")
	if err != nil { return nil, err }
	defer rows.Close()

	var results []skills.Skill
	for rows.Next() {
		var s skills.Skill
		rows.Scan(&s.ID, &s.Name, &s.Description, &s.CreatedBy, &s.Active, &s.CreatedAt)
		results = append(results, s)
	}
	return results, nil
}

func (e *SQLiteSkillEngine) GetVersion(ctx context.Context, skillID, version string) (*skills.SkillVersion, error) {
	row := e.db.QueryRowContext(ctx, "SELECT skill_id, version, parent, code_path, entry_point, score, active, created_at FROM skill_versions WHERE skill_id = ? AND version = ?", skillID, version)
	var v skills.SkillVersion
	err := row.Scan(&v.SkillID, &v.Version, &v.Parent, &v.CodePath, &v.EntryPoint, &v.Score, &v.Active, &v.CreatedAt)
	if err != nil { return nil, err }
	return &v, nil
}

func (e *SQLiteSkillEngine) ListVersions(ctx context.Context, skillID string) ([]skills.SkillVersion, error) {
	rows, err := e.db.QueryContext(ctx, "SELECT skill_id, version, parent, code_path, entry_point, score, active, created_at FROM skill_versions WHERE skill_id = ? ORDER BY created_at DESC", skillID)
	if err != nil { return nil, err }
	defer rows.Close()

	var results []skills.SkillVersion
	for rows.Next() {
		var v skills.SkillVersion
		rows.Scan(&v.SkillID, &v.Version, &v.Parent, &v.CodePath, &v.EntryPoint, &v.Score, &v.Active, &v.CreatedAt)
		results = append(results, v)
	}
	return results, nil
}

func (e *SQLiteSkillEngine) Execute(ctx context.Context, skillID string, input map[string]any) (map[string]any, error) {
	// 获取当前活跃版本
	row := e.db.QueryRowContext(ctx, "SELECT code_path, entry_point FROM skill_versions WHERE skill_id = ? AND active = 1", skillID)
	var codePath, entryPoint string
	if err := row.Scan(&codePath, &entryPoint); err != nil {
		return nil, fmt.Errorf("active version not found for skill: %s", skillID)
	}
	// TODO: 调用沙箱执行逻辑
	return nil, fmt.Errorf("execution link needs sandbox integration")
}

var _ skills.SkillEngine = (*SQLiteSkillEngine)(nil)
