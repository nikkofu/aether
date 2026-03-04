package email

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nikkofu/aether/internal/adapters/capabilities"
)

// EmailCapability 实现了受审计和限速保护的邮件能力。
type EmailCapability struct {
	mu         sync.Mutex
	lastSentTs map[string]time.Time // org_id -> last_sent_time
}

func NewEmailCapability() *EmailCapability {
	return &EmailCapability{
		lastSentTs: make(map[string]time.Time),
	}
}

func (c *EmailCapability) Name() string { return "email_service" }

func (c *EmailCapability) Execute(ctx context.Context, req capabilities.CapabilityRequest) (capabilities.CapabilityResponse, error) {
	action, _ := req.Params["action"].(string)

	switch action {
	case "send":
		// 1. 限速检查 (每个组织每 10 秒仅限发送一封)
		c.mu.Lock()
		last, exists := c.lastSentTs[req.OrgID]
		if exists && time.Since(last) < 10*time.Second {
			c.mu.Unlock()
			return capabilities.CapabilityResponse{Success: false, Error: "rate limit exceeded"}, nil
		}
		c.lastSentTs[req.OrgID] = time.Now()
		c.mu.Unlock()

		to, _ := req.Params["to"].(string)
		subject, _ := req.Params["subject"].(string)
		if to == "" || subject == "" { return capabilities.CapabilityResponse{Success: false, Error: "missing params"}, nil }

		// 2. 模拟发送逻辑 (实际应调用 SMTP 库)
		fmt.Printf("[EMAIL SENT] To: %s, Sub: %s (Org: %s)\n", to, subject, req.OrgID)
		return capabilities.CapabilityResponse{Success: true}, nil

	case "read":
		// 模拟读取逻辑
		return capabilities.CapabilityResponse{
			Success: true,
			Data: map[string]any{
				"inbox": []string{"Message 1 from CEO", "System alert: task success"},
			},
		}, nil

	default:
		return capabilities.CapabilityResponse{Success: false, Error: "unsupported action"}, nil
	}
}
