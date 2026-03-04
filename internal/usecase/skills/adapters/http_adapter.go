package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nikkofu/aether/pkg/audit"
	"github.com/nikkofu/aether/pkg/logging"
)

// HTTPToolAdapter 实现了将远程 HTTP 接口包装为技能的能力。
type HTTPToolAdapter struct {
	client *http.Client
	logger logging.Logger
	audit  audit.Logger
}

func NewHTTPToolAdapter(l logging.Logger, a audit.Logger) *HTTPToolAdapter {
	return &HTTPToolAdapter{
		client: &http.Client{Timeout: 10 * time.Second},
		logger: l,
		audit:  a,
	}
}

// ExecuteCall 执行实际的 HTTP 调用并记录审计。
func (a *HTTPToolAdapter) ExecuteCall(ctx context.Context, orgID string, name string, endpoint string, input map[string]any) (map[string]any, error) {
	start := time.Now()

	inputBytes, _ := json.Marshal(input)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(inputBytes))
	if err != nil { return nil, err }
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("HTTP 调用失败: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	// 记录审计与日志
	if a.audit != nil {
		a.audit.Log(ctx, orgID, audit.EventEconomyCharge, "HTTP 技能调用", map[string]any{
			"tool": name, "latency_ms": duration.Milliseconds(), "status": resp.StatusCode,
		})
	}

	return result, nil
}
