package alarm

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nikkofu/aether/internal/infrastructure/capabilities"
)

type AlarmCapability struct {
	db *sql.DB
}

func NewAlarmCapability(db *sql.DB) (*AlarmCapability, error) {
	c := &AlarmCapability{db: db}
	if err := c.init(context.Background()); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *AlarmCapability) init(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS alarms (
		id TEXT PRIMARY KEY,
		org_id TEXT NOT NULL,
		alarm_time DATETIME NOT NULL,
		message TEXT,
		status TEXT DEFAULT 'pending',
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_alarms_org ON alarms(org_id);
	`
	_, err := c.db.ExecContext(ctx, query)
	return err
}

func (c *AlarmCapability) Name() string { return "alarm_service" }

func (c *AlarmCapability) Execute(ctx context.Context, req capabilities.CapabilityRequest) (capabilities.CapabilityResponse, error) {
	action, _ := req.Params["action"].(string)

	switch action {
	case "create":
		id := fmt.Sprintf("alm-%d", time.Now().UnixNano())
		timeStr, _ := req.Params["time"].(string)
		msg, _ := req.Params["message"].(string)

		alarmTime, _ := time.Parse(time.RFC3339, timeStr)

		query := `INSERT INTO alarms (id, org_id, alarm_time, message, created_at) VALUES (?, ?, ?, ?, ?)`
		_, err := c.db.ExecContext(ctx, query, id, req.OrgID, alarmTime, msg, time.Now())
		if err != nil {
			return capabilities.CapabilityResponse{Success: false, Error: err.Error()}, nil
		}
		return capabilities.CapabilityResponse{Success: true, Data: map[string]any{"alarm_id": id}}, nil

	case "cancel":
		id, _ := req.Params["alarm_id"].(string)
		_, err := c.db.ExecContext(ctx, "UPDATE alarms SET status = 'cancelled' WHERE id = ? AND org_id = ?", id, req.OrgID)
		if err != nil {
			return capabilities.CapabilityResponse{Success: false, Error: err.Error()}, nil
		}
		return capabilities.CapabilityResponse{Success: true}, nil

	case "list":
		query := `SELECT id, alarm_time, message, status FROM alarms WHERE org_id = ? AND status = 'pending' ORDER BY alarm_time ASC`
		rows, err := c.db.QueryContext(ctx, query, req.OrgID)
		if err != nil {
			return capabilities.CapabilityResponse{Success: false, Error: err.Error()}, nil
		}
		defer rows.Close()

		var results []map[string]any
		for rows.Next() {
			var id, msg, status string
			var t time.Time
			rows.Scan(&id, &t, &msg, &status)
			results = append(results, map[string]any{
				"id": id, "time": t, "message": msg, "status": status,
			})
		}
		return capabilities.CapabilityResponse{Success: true, Data: map[string]any{"alarms": results}}, nil

	default:
		return capabilities.CapabilityResponse{Success: false, Error: "unsupported action"}, nil
	}
}
