package rbac

import (
	"context"
	"database/sql"
)

type SQLiteRBAC struct {
	db *sql.DB
}

func NewSQLiteRBAC(db *sql.DB) (*SQLiteRBAC, error) {
	r := &SQLiteRBAC{db: db}
	if err := r.init(context.Background()); err != nil { return nil, err }
	return r, nil
}

func (r *SQLiteRBAC) init(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS user_roles (
			user_id TEXT NOT NULL,
			org_id TEXT NOT NULL,
			role TEXT NOT NULL,
			PRIMARY KEY (user_id, org_id)
		);`,
		`CREATE TABLE IF NOT EXISTS role_permissions (
			role TEXT NOT NULL,
			permission TEXT NOT NULL,
			PRIMARY KEY (role, permission)
		);`,
	}
	for _, q := range queries {
		if _, err := r.db.ExecContext(ctx, q); err != nil { return err }
	}
	return r.seedPermissions(ctx)
}

func (r *SQLiteRBAC) seedPermissions(ctx context.Context) error {
	matrix := map[Role][]Permission{
		RoleAdmin:    {PermCreateProposal, PermVoteProposal, PermModifyEconomy, PermViewAudit, PermSpawnAgent},
		RoleOperator: {PermCreateProposal, PermVoteProposal, PermSpawnAgent},
		RoleViewer:   {PermViewAudit},
		RoleAgent:    {PermVoteProposal},
	}
	for role, perms := range matrix {
		for _, p := range perms {
			r.db.ExecContext(ctx, `INSERT OR IGNORE INTO role_permissions (role, permission) VALUES (?, ?)`, string(role), string(p))
		}
	}
	return nil
}

func (r *SQLiteRBAC) AssignRole(userID string, role Role, orgID string) error {
	_, err := r.db.Exec(`INSERT INTO user_roles (user_id, org_id, role) VALUES (?, ?, ?) ON CONFLICT(user_id, org_id) DO UPDATE SET role=excluded.role`, userID, orgID, string(role))
	return err
}

func (r *SQLiteRBAC) CheckPermission(userID string, perm Permission, orgID string) bool {
	query := `
	SELECT COUNT(*) FROM user_roles ur
	JOIN role_permissions rp ON ur.role = rp.role
	WHERE ur.user_id = ? AND ur.org_id = ? AND rp.permission = ?`
	var count int
	r.db.QueryRow(query, userID, orgID, string(perm)).Scan(&count)
	return count > 0
}
