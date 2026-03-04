package cluster

import (
	"context"
	"time"

	"github.com/nikkofu/aether/internal/agent"
	"github.com/nikkofu/aether/internal/logging"
)

const HeartbeatInterval = 3 * time.Second
const HeartbeatTimeout = 10 * time.Second

// StartWorkerHeartbeat 启动心跳发送协程。
func StartWorkerHeartbeat(ctx context.Context, b agent.Bus, role, workerID string, logger logging.Logger) {
	ticker := time.NewTicker(HeartbeatInterval)
	if logger != nil {
		logger.Info(ctx, "心跳服务已启动", logging.String("worker_id", workerID), logging.Duration("interval", HeartbeatInterval))
	}

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				b.Publish(ctx, agent.Message{
					From:      workerID,
					To:        "leader",
					Type:      "heartbeat",
					Timestamp: time.Now(),
					Payload: map[string]any{
						"role":      role,
						"worker_id": workerID,
					},
				})
			}
		}
	}()
}
