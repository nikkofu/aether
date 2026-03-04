package constitution

import (
	"context"
	"database/sql"
	"time"
)

// SQLiteConstitution 实现了基于 SQLite 的宪法管理。
type SQLiteConstitution struct {
	db *sql.DB
}

func NewSQLiteConstitution(db *sql.DB) (*SQLiteConstitution, error) {
	c := &SQLiteConstitution{db: db}
	if err := c.init(context.Background()); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *SQLiteConstitution) init(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS constitutional_rules (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT,
		immutable BOOLEAN DEFAULT 0,
		created_at DATETIME NOT NULL
	);`
	_, err := c.db.ExecContext(ctx, query)
	return err
}

func (c *SQLiteConstitution) AddRule(rule ConstitutionalRule) error {
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now()
	}
	query := `INSERT INTO constitutional_rules (id, title, description, immutable, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := c.db.Exec(query, rule.ID, rule.Title, rule.Description, rule.Immutable, rule.CreatedAt)
	return err
}

func (c *SQLiteConstitution) GetRule(id string) (*ConstitutionalRule, error) {
	query := `SELECT id, title, description, immutable, created_at FROM constitutional_rules WHERE id = ?`
	var r ConstitutionalRule
	err := c.db.QueryRow(query, id).Scan(&r.ID, &r.Title, &r.Description, &r.Immutable, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *SQLiteConstitution) ListRules() ([]ConstitutionalRule, error) {
	rows, err := c.db.Query("SELECT id, title, description, immutable, created_at FROM constitutional_rules")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []ConstitutionalRule
	for rows.Next() {
		var r ConstitutionalRule
		if err := rows.Scan(&r.ID, &r.Title, &r.Description, &r.Immutable, &r.CreatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// ValidatePolicyChange 根据宪法条款校验变更。
func (c *SQLiteConstitution) ValidatePolicyChange(policyType string, newValue any) error {
	// 示例宪法规则校验逻辑
	switch policyType {
	case "max_concurrency":
		val, ok := newValue.(int)
		if ok && val > 100 {
			return &ErrRuleViolation{RuleID: "RULE_RESOURCE_LIMIT", Message: "并发数不能超过系统硬上限 100"}
		}
	case "budget_limit":
		val, ok := newValue.(float64)
		if ok && val < 0 {
			return &ErrRuleViolation{RuleID: "RULE_FISCAL_SAFETY", Message: "预算上限不能为负数"}
		}
	}
	return nil
}

var _ Constitution = (*SQLiteConstitution)(nil)
