package economy

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestNewSQLiteLedgerMigratesLegacySchema(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	legacyStatements := []string{
		`CREATE TABLE accounts (
			agent_id TEXT PRIMARY KEY,
			balance REAL DEFAULT 0.0,
			reputation REAL DEFAULT 0.0,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE transactions (
			id TEXT PRIMARY KEY,
			from_agent TEXT,
			to_agent TEXT,
			amount REAL,
			type TEXT,
			created_at DATETIME NOT NULL
		);`,
		`INSERT INTO accounts (agent_id, balance, reputation, updated_at) VALUES ('agent-1', 5.0, 7.0, CURRENT_TIMESTAMP);`,
		`INSERT INTO transactions (id, from_agent, to_agent, amount, type, created_at) VALUES ('tx-1', 'agent-1', 'system', 1.0, 'cost', CURRENT_TIMESTAMP);`,
	}
	for _, stmt := range legacyStatements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("prepare legacy schema: %v", err)
		}
	}

	ledger, err := NewSQLiteLedger(db)
	if err != nil {
		t.Fatalf("migrate ledger: %v", err)
	}

	account, err := ledger.GetAccount(context.Background(), "default", "agent-1")
	if err != nil {
		t.Fatalf("load migrated account: %v", err)
	}
	if account.OrgID != "default" {
		t.Fatalf("expected migrated org_id=default, got %s", account.OrgID)
	}

	if err := ledger.UpdateBalance(context.Background(), "default", "agent-1", 2.0, 1.0); err != nil {
		t.Fatalf("update migrated account: %v", err)
	}

	transactions, err := ledger.ListTransactions(context.Background(), "default")
	if err != nil {
		t.Fatalf("load migrated transactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Fatalf("expected migrated transactions, got %d", len(transactions))
	}
	if transactions[0].OrgID != "default" {
		t.Fatalf("expected migrated transaction org_id=default, got %s", transactions[0].OrgID)
	}
}
