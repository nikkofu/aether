package diary

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nikkofu/aether/internal/infrastructure/capabilities"
)

type DiaryCapability struct {
	db *sql.DB
}

func NewDiaryCapability(db *sql.DB) (*DiaryCapability, error) {
	c := &DiaryCapability{db: db}
	if err := c.init(context.Background()); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *DiaryCapability) init(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS diaries (
		id TEXT PRIMARY KEY,
		org_id TEXT NOT NULL,
		content TEXT NOT NULL,
		mood TEXT,
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_diaries_org ON diaries(org_id);
	`
	_, err := c.db.ExecContext(ctx, query)
	return err
}

func (c *DiaryCapability) Name() string { return "diary_service" }

func (c *DiaryCapability) Execute(ctx context.Context, req capabilities.CapabilityRequest) (capabilities.CapabilityResponse, error) {
	action, _ := req.Params["action"].(string)

	switch action {
	case "write":
		id := fmt.Sprintf("dry-%d", time.Now().UnixNano())
		content, _ := req.Params["content"].(string)
		mood, _ := req.Params["mood"].(string)

		query := `INSERT INTO diaries (id, org_id, content, mood, created_at) VALUES (?, ?, ?, ?, ?)`
		_, err := c.db.ExecContext(ctx, query, id, req.OrgID, content, mood, time.Now())
		if err != nil {
			return capabilities.CapabilityResponse{Success: false, Error: err.Error()}, nil
		}
		return capabilities.CapabilityResponse{Success: true, Data: map[string]any{"diary_id": id}}, nil

	case "read":
		query := `SELECT id, content, mood, created_at FROM diaries WHERE org_id = ? ORDER BY created_at DESC`
		rows, err := c.db.QueryContext(ctx, query, req.OrgID)
		if err != nil {
			return capabilities.CapabilityResponse{Success: false, Error: err.Error()}, nil
		}
		defer rows.Close()

		var results []map[string]any
		for rows.Next() {
			var id, content, mood string
			var created time.Time
			rows.Scan(&id, &content, &mood, &created)
			results = append(results, map[string]any{
				"id": id, "content": content, "mood": mood, "date": created,
			})
		}
		return capabilities.CapabilityResponse{Success: true, Data: map[string]any{"diaries": results}}, nil

	default:
		return capabilities.CapabilityResponse{Success: false, Error: "unsupported action"}, nil
	}
}
