package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/google/uuid"
	"github.com/nikkofu/aether/internal/economy"
	"github.com/nikkofu/aether/internal/logging"
)

// ExecuteInSandbox 在绝对隔离的子进程中执行技能。
func ExecuteInSandbox(ctx context.Context, logger logging.Logger, ledger economy.Ledger, orgID, agentID, codePath, entryPoint string, input map[string]any) (map[string]any, error) {
	childCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 1. 经济前置扣费
	const sandboxBaseCost = 0.01
	if ledger != nil {
		ledger.UpdateBalance(ctx, orgID, agentID, -sandboxBaseCost, 0)
	}

	// 2. 环境隔离铁律：清空所有敏感环境变量
	// 仅允许传递非敏感的基本路径
	safeEnv := []string{
		"PATH=/usr/bin:/usr/local/bin:/bin",
		"HOME=/tmp",
		"TMPDIR=/tmp",
	}

	inputBytes, _ := json.Marshal(input)
	cmd := exec.CommandContext(childCtx, codePath, entryPoint)
	cmd.Stdin = bytes.NewReader(inputBytes)
	cmd.Env = safeEnv // 强制使用白名单环境
	cmd.Dir = "/tmp"  // 限制工作目录

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	// 3. 经济反馈与审计
	if err != nil {
		if ledger != nil { ledger.UpdateBalance(ctx, orgID, agentID, 0, -0.5) }
		return nil, fmt.Errorf("沙箱隔离执行失败: %s", stderr.String())
	}

	if ledger != nil {
		bonus := 0.0
		if duration < 1*time.Second { bonus = 0.02 }
		ledger.UpdateBalance(ctx, orgID, agentID, bonus, 0.5)
		ledger.AddTransaction(ctx, economy.Transaction{
			ID: uuid.New().String(), OrgID: orgID, From: "system_sandbox", To: agentID, Amount: bonus, Type: "reward",
		})
	}

	var output map[string]any
	json.Unmarshal(stdout.Bytes(), &output)
	return output, nil
}
