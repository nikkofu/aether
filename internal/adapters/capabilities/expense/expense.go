package expense

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nikkofu/aether/internal/adapters/capabilities"
)

type ExpenseCapability struct {
	db *sql.DB
}

func NewExpenseCapability(db *sql.DB) (*ExpenseCapability, error) {
	c := &ExpenseCapability{db: db}
	if err := c.init(context.Background()); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *ExpenseCapability) init(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS expenses (
		id TEXT PRIMARY KEY,
		org_id TEXT NOT NULL,
		amount REAL NOT NULL,
		category TEXT NOT NULL,
		description TEXT,
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_expenses_org ON expenses(org_id);
	`
	_, err := c.db.ExecContext(ctx, query)
	return err
}

func (c *ExpenseCapability) Name() string { return "expense_manager" }

func (c *ExpenseCapability) Execute(ctx context.Context, req capabilities.CapabilityRequest) (capabilities.CapabilityResponse, error) {
	action, _ := req.Params["action"].(string)

	switch action {
	case "record":
		id := fmt.Sprintf("exp-%d", time.Now().UnixNano())
		amount, _ := req.Params["amount"].(float64)
		category, _ := req.Params["category"].(string)
		desc, _ := req.Params["description"].(string)

		query := `INSERT INTO expenses (id, org_id, amount, category, description, created_at) VALUES (?, ?, ?, ?, ?, ?)`
		_, err := c.db.ExecContext(ctx, query, id, req.OrgID, amount, category, desc, time.Now())
		if err != nil {
			return capabilities.CapabilityResponse{Success: false, Error: err.Error()}, nil
		}
		return capabilities.CapabilityResponse{Success: true, Data: map[string]any{"expense_id": id}}, nil

	case "list":
		query := `SELECT id, amount, category, description, created_at FROM expenses WHERE org_id = ? ORDER BY created_at DESC`
		rows, err := c.db.QueryContext(ctx, query, req.OrgID)
		if err != nil {
			return capabilities.CapabilityResponse{Success: false, Error: err.Error()}, nil
		}
		defer rows.Close()

		var results []map[string]any
		for rows.Next() {
			var id, cat, desc string
			var amount float64
			var created time.Time
			rows.Scan(&id, &amount, &cat, &desc, &created)
			results = append(results, map[string]any{
				"id": id, "amount": amount, "category": cat, "description": desc, "date": created,
			})
		}
		return capabilities.CapabilityResponse{Success: true, Data: map[string]any{"expenses": results}}, nil

	default:
		return capabilities.CapabilityResponse{Success: false, Error: "unsupported action"}, nil
	}
}
