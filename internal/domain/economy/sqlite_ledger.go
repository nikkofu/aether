package economy

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteLedger struct {
	db *sql.DB
}

func NewSQLiteLedger(db *sql.DB) (*SQLiteLedger, error) {
	l := &SQLiteLedger{db: db}
	if err := l.init(context.Background()); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *SQLiteLedger) init(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS accounts (
			agent_id TEXT,
			org_id TEXT,
			balance REAL DEFAULT 0.0,
			reputation REAL DEFAULT 0.0,
			updated_at DATETIME NOT NULL,
			PRIMARY KEY (agent_id, org_id)
		);`,
		`CREATE TABLE IF NOT EXISTS transactions (
			id TEXT PRIMARY KEY,
			org_id TEXT NOT NULL,
			from_agent TEXT,
			to_agent TEXT,
			amount REAL,
			type TEXT,
			created_at DATETIME NOT NULL
		);`,
	}
	for _, q := range queries {
		if _, err := l.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	if err := l.migrateLegacyAccounts(ctx); err != nil {
		return err
	}
	if err := l.migrateLegacyTransactions(ctx); err != nil {
		return err
	}
	if _, err := l.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_accounts_org_rep ON accounts(org_id, reputation DESC);`); err != nil {
		return err
	}
	if _, err := l.db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_agent_org ON accounts(agent_id, org_id);`); err != nil {
		return err
	}
	return nil
}

func (l *SQLiteLedger) migrateLegacyAccounts(ctx context.Context) error {
	if l == nil || l.db == nil {
		return fmt.Errorf("ledger database is not initialized")
	}

	hasOrgID, err := l.tableHasColumn(ctx, "accounts", "org_id")
	if err != nil || hasOrgID {
		return err
	}

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	steps := []string{
		`ALTER TABLE accounts RENAME TO accounts_legacy;`,
		`CREATE TABLE accounts (
			agent_id TEXT,
			org_id TEXT NOT NULL DEFAULT 'default',
			balance REAL DEFAULT 0.0,
			reputation REAL DEFAULT 0.0,
			updated_at DATETIME NOT NULL,
			PRIMARY KEY (agent_id, org_id)
		);`,
		`INSERT INTO accounts (agent_id, org_id, balance, reputation, updated_at)
		 SELECT agent_id, 'default', balance, reputation, updated_at
		 FROM accounts_legacy;`,
		`DROP TABLE accounts_legacy;`,
	}
	for _, step := range steps {
		if _, err := tx.ExecContext(ctx, step); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (l *SQLiteLedger) migrateLegacyTransactions(ctx context.Context) error {
	if l == nil || l.db == nil {
		return fmt.Errorf("ledger database is not initialized")
	}

	hasOrgID, err := l.tableHasColumn(ctx, "transactions", "org_id")
	if err != nil || hasOrgID {
		return err
	}

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	steps := []string{
		`ALTER TABLE transactions RENAME TO transactions_legacy;`,
		`CREATE TABLE transactions (
			id TEXT PRIMARY KEY,
			org_id TEXT NOT NULL,
			from_agent TEXT,
			to_agent TEXT,
			amount REAL,
			type TEXT,
			created_at DATETIME NOT NULL
		);`,
		`INSERT INTO transactions (id, org_id, from_agent, to_agent, amount, type, created_at)
		 SELECT id, 'default', from_agent, to_agent, amount, type, created_at
		 FROM transactions_legacy;`,
		`DROP TABLE transactions_legacy;`,
	}
	for _, step := range steps {
		if _, err := tx.ExecContext(ctx, step); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (l *SQLiteLedger) tableHasColumn(ctx context.Context, tableName, columnName string) (bool, error) {
	rows, err := l.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (l *SQLiteLedger) GetAccount(ctx context.Context, orgID string, agentID string) (*Account, error) {
	query := `SELECT agent_id, org_id, balance, reputation, updated_at FROM accounts WHERE org_id = ? AND agent_id = ?`
	row := l.db.QueryRowContext(ctx, query, orgID, agentID)
	var acc Account
	err := row.Scan(&acc.AgentID, &acc.OrgID, &acc.Balance, &acc.Reputation, &acc.UpdatedAt)
	if err == sql.ErrNoRows {
		return &Account{AgentID: agentID, OrgID: orgID, Balance: 10.0, Reputation: 10.0}, nil
	}
	return &acc, err
}

func (l *SQLiteLedger) UpdateBalance(ctx context.Context, orgID string, agentID string, delta float64, repDelta float64) error {
	query := `
	INSERT INTO accounts (agent_id, org_id, balance, reputation, updated_at)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(agent_id, org_id) DO UPDATE SET
		balance = balance + excluded.balance,
		reputation = reputation + excluded.reputation,
		updated_at = excluded.updated_at`
	_, err := l.db.ExecContext(ctx, query, agentID, orgID, delta, repDelta, time.Now())
	return err
}

func (l *SQLiteLedger) AddTransaction(ctx context.Context, tx Transaction) error {
	if tx.CreatedAt.IsZero() {
		tx.CreatedAt = time.Now()
	}
	query := `INSERT INTO transactions (id, org_id, from_agent, to_agent, amount, type, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := l.db.ExecContext(ctx, query, tx.ID, tx.OrgID, tx.From, tx.To, tx.Amount, tx.Type, tx.CreatedAt)
	return err
}

func (l *SQLiteLedger) TopAgentsByReputation(ctx context.Context, orgID string, limit int) ([]Account, error) {
	query := `SELECT agent_id, org_id, balance, reputation, updated_at FROM accounts WHERE org_id = ? ORDER BY reputation DESC LIMIT ?`
	rows, err := l.db.QueryContext(ctx, query, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []Account
	for rows.Next() {
		var acc Account
		rows.Scan(&acc.AgentID, &acc.OrgID, &acc.Balance, &acc.Reputation, &acc.UpdatedAt)
		results = append(results, acc)
	}
	return results, nil
}

func (l *SQLiteLedger) ListTransactions(ctx context.Context, orgID string) ([]Transaction, error) {
	query := `SELECT id, org_id, from_agent, to_agent, amount, type, created_at FROM transactions WHERE org_id = ? ORDER BY created_at DESC`
	rows, err := l.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []Transaction
	for rows.Next() {
		var tx Transaction
		rows.Scan(&tx.ID, &tx.OrgID, &tx.From, &tx.To, &tx.Amount, &tx.Type, &tx.CreatedAt)
		results = append(results, tx)
	}
	return results, nil
}

func (l *SQLiteLedger) ApplyReputationDecay(ctx context.Context, orgID string, rate float64) error {
	query := `UPDATE accounts SET reputation = reputation * (1.0 - ?) WHERE org_id = ?`
	_, err := l.db.ExecContext(ctx, query, rate, orgID)
	return err
}

func (l *SQLiteLedger) BurnExcessTokens(ctx context.Context, orgID string, maxTotalSupply float64) error {
	var total float64
	l.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(balance), 0) FROM accounts WHERE org_id = ?`, orgID).Scan(&total)
	if total > maxTotalSupply {
		ratio := maxTotalSupply / total
		l.db.ExecContext(ctx, `UPDATE accounts SET balance = balance * ? WHERE org_id = ?`, ratio, orgID)
	}
	return nil
}

var _ Ledger = (*SQLiteLedger)(nil)
