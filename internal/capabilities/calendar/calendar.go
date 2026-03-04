package calendar

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nikkofu/aether/internal/capabilities"
)

type CalendarCapability struct {
	db *sql.DB
}

func NewCalendarCapability(db *sql.DB) (*CalendarCapability, error) {
	c := &CalendarCapability{db: db}
	if err := c.init(context.Background()); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *CalendarCapability) init(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS calendar_events (
		id TEXT PRIMARY KEY,
		org_id TEXT NOT NULL,
		title TEXT NOT NULL,
		start_time DATETIME NOT NULL,
		end_time DATETIME NOT NULL,
		description TEXT,
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_calendar_org ON calendar_events(org_id);
	`
	_, err := c.db.ExecContext(ctx, query)
	return err
}

func (c *CalendarCapability) Name() string { return "calendar_service" }

func (c *CalendarCapability) Execute(ctx context.Context, req capabilities.CapabilityRequest) (capabilities.CapabilityResponse, error) {
	action, _ := req.Params["action"].(string)

	switch action {
	case "create_event":
		id := fmt.Sprintf("ev-%d", time.Now().UnixNano())
		title, _ := req.Params["title"].(string)
		startStr, _ := req.Params["start"].(string)
		endStr, _ := req.Params["end"].(string)
		desc, _ := req.Params["description"].(string)

		start, _ := time.Parse(time.RFC3339, startStr)
		end, _ := time.Parse(time.RFC3339, endStr)

		query := `INSERT INTO calendar_events (id, org_id, title, start_time, end_time, description, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
		_, err := c.db.ExecContext(ctx, query, id, req.OrgID, title, start, end, desc, time.Now())
		if err != nil {
			return capabilities.CapabilityResponse{Success: false, Error: err.Error()}, nil
		}
		return capabilities.CapabilityResponse{Success: true, Data: map[string]any{"event_id": id}}, nil

	case "list_events":
		query := `SELECT id, title, start_time, end_time, description FROM calendar_events WHERE org_id = ? ORDER BY start_time ASC`
		rows, err := c.db.QueryContext(ctx, query, req.OrgID)
		if err != nil {
			return capabilities.CapabilityResponse{Success: false, Error: err.Error()}, nil
		}
		defer rows.Close()

		var events []map[string]any
		for rows.Next() {
			var id, title, desc string
			var start, end time.Time
			rows.Scan(&id, &title, &start, &end, &desc)
			events = append(events, map[string]any{
				"id": id, "title": title, "start": start, "end": end, "description": desc,
			})
		}
		return capabilities.CapabilityResponse{Success: true, Data: map[string]any{"events": events}}, nil

	default:
		return capabilities.CapabilityResponse{Success: false, Error: "unsupported action"}, nil
	}
}
