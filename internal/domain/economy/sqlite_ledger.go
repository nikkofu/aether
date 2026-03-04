package economy

import (
	"context"
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteLedger struct {
	db *sql.DB
}

func NewSQLiteLedger(db *sql.DB) (*SQLiteLedger, error) {
	l := &SQLiteLedger{db: db}
	if err := l.init(context.Background()); err != nil { return nil, err }
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
		`CREATE INDEX IF NOT EXISTS idx_accounts_org_rep ON accounts(org_id, reputation DESC);`,
	}
	for _, q := range queries {
		if _, err := l.db.ExecContext(ctx, q); err != nil { return err }
	}
	return nil
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
	if tx.CreatedAt.IsZero() { tx.CreatedAt = time.Now() }
	query := `INSERT INTO transactions (id, org_id, from_agent, to_agent, amount, type, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := l.db.ExecContext(ctx, query, tx.ID, tx.OrgID, tx.From, tx.To, tx.Amount, tx.Type, tx.CreatedAt)
	return err
}

func (l *SQLiteLedger) TopAgentsByReputation(ctx context.Context, orgID string, limit int) ([]Account, error) {
	query := `SELECT agent_id, org_id, balance, reputation, updated_at FROM accounts WHERE org_id = ? ORDER BY reputation DESC LIMIT ?`
	rows, err := l.db.QueryContext(ctx, query, orgID, limit)
	if err != nil { return nil, err }
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
	if err != nil { return nil, err }
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
