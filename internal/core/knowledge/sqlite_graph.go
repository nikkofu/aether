package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteGraph 实现了基于 SQLite 的多租户知识图谱。
type SQLiteGraph struct {
	db *sql.DB
}

func NewSQLiteGraph(db *sql.DB) (*SQLiteGraph, error) {
	if db == nil { return nil, fmt.Errorf("db required") }
	g := &SQLiteGraph{db: db}
	if err := g.init(context.Background()); err != nil { return nil, err }
	return g, nil
}

func (g *SQLiteGraph) init(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS entities (
			id TEXT PRIMARY KEY,
			org_id TEXT NOT NULL,
			type TEXT NOT NULL,
			name TEXT NOT NULL,
			metadata TEXT,
			created_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_entities_org_type ON entities(org_id, type);`,
		`CREATE TABLE IF NOT EXISTS relations (
			id TEXT PRIMARY KEY,
			org_id TEXT NOT NULL,
			from_id TEXT NOT NULL,
			to_id TEXT NOT NULL,
			type TEXT NOT NULL,
			metadata TEXT,
			created_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_relations_org ON relations(org_id);`,
	}
	for _, q := range queries {
		if _, err := g.db.ExecContext(ctx, q); err != nil { return err }
	}
	return nil
}

// 注意：实体 ID 应该是全局唯一的，或者通过 (id, org_id) 复合主键。
// 此处为了生产一致性，我们在查询中强制 org_id。

func (g *SQLiteGraph) AddEntity(ctx context.Context, e Entity, orgID string) error {
	if e.CreatedAt.IsZero() { e.CreatedAt = time.Now() }
	metaJSON, _ := json.Marshal(e.Metadata)
	query := `INSERT INTO entities (id, org_id, type, name, metadata, created_at) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := g.db.ExecContext(ctx, query, e.ID, orgID, e.Type, e.Name, string(metaJSON), e.CreatedAt)
	return err
}

func (g *SQLiteGraph) AddRelation(ctx context.Context, r Relation, orgID string) error {
	if r.CreatedAt.IsZero() { r.CreatedAt = time.Now() }
	metaJSON, _ := json.Marshal(r.Metadata)
	query := `INSERT INTO relations (id, org_id, from_id, to_id, type, metadata, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := g.db.ExecContext(ctx, query, r.ID, orgID, r.FromID, r.ToID, r.Type, string(metaJSON), r.CreatedAt)
	return err
}

func (g *SQLiteGraph) GetEntity(ctx context.Context, id string) (*Entity, error) {
	query := `SELECT id, type, name, metadata, created_at FROM entities WHERE id = ?`
	var e Entity
	var metaStr string
	err := g.db.QueryRowContext(ctx, query, id).Scan(&e.ID, &e.Type, &e.Name, &metaStr, &e.CreatedAt)
	if err != nil { return nil, err }
	json.Unmarshal([]byte(metaStr), &e.Metadata)
	return &e, nil
}

func (g *SQLiteGraph) QueryByType(ctx context.Context, orgID string, entityType string) ([]Entity, error) {
	query := `SELECT id, type, name, metadata, created_at FROM entities WHERE org_id = ? AND type = ? ORDER BY created_at DESC`
	rows, err := g.db.QueryContext(ctx, query, orgID, entityType)
	if err != nil { return nil, err }
	defer rows.Close()

	var results []Entity
	for rows.Next() {
		var e Entity
		var metaStr string
		rows.Scan(&e.ID, &e.Type, &e.Name, &metaStr, &e.CreatedAt)
		json.Unmarshal([]byte(metaStr), &e.Metadata)
		results = append(results, e)
	}
	return results, nil
}

func (g *SQLiteGraph) GetRelations(ctx context.Context, orgID string, id string) ([]Relation, error) {
	query := `SELECT id, from_id, to_id, type, metadata, created_at FROM relations WHERE org_id = ? AND (from_id = ? OR to_id = ?)`
	rows, err := g.db.QueryContext(ctx, query, orgID, id, id)
	if err != nil { return nil, err }
	defer rows.Close()

	var results []Relation
	for rows.Next() {
		var r Relation
		var metaStr string
		rows.Scan(&r.ID, &r.FromID, &r.ToID, &r.Type, &metaStr, &r.CreatedAt)
		json.Unmarshal([]byte(metaStr), &r.Metadata)
		results = append(results, r)
	}
	return results, nil
}
