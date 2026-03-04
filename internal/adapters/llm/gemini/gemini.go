package gemini

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/nikkofu/aether/internal/adapters/llm"
)

// Adapter 实现 cli_adapters.Adapter 接口，用于调用本地 gemini CLI 命令。
type Adapter struct {
	binaryPath string
}

// NewAdapter 创建一个新的 Gemini 适配器。
func NewAdapter() *Adapter {
	return &Adapter{binaryPath: "gemini"}
}

// NewAdapterWithBinary 指定特定的 gemini 二进制文件路径。
func NewAdapterWithBinary(path string) *Adapter {
	return &Adapter{binaryPath: path}
}

// Name 返回此适配器的名称。
func (a *Adapter) Name() string {
	return "gemini"
}

// Execute 阻塞式执行命令并返回完整输出。
func (a *Adapter) Execute(ctx context.Context, prompt string) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("prompt 不能为空")
	}

	// 使用 CommandContext 确保支持 context 的取消和超时。
	cmd := exec.CommandContext(ctx, a.binaryPath, prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			errMsg := strings.TrimSpace(stderr.String())
			if errMsg == "" {
				errMsg = exitErr.Error()
			}
			return "", fmt.Errorf("gemini 命令执行失败 (退出码 %d): %s", exitErr.ExitCode(), errMsg)
		}
		return "", fmt.Errorf("无法启动 gemini 命令: %w", err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// Stream 实现流式输出。
// 它通过 StdoutPipe 实时捕获输出，并使用 bufio.Scanner 按字符触发回调。
func (a *Adapter) Stream(ctx context.Context, prompt string, onToken cli_adapters.TokenCallback) error {
	if prompt == "" {
		return fmt.Errorf("prompt 不能为空")
	}

	// 1. 使用 CommandContext 绑定生命周期
	cmd := exec.CommandContext(ctx, a.binaryPath, prompt)

	// 2. 获取 Stdout 管道用于实时读取
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("无法创建 stdout 管道: %w", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// 3. 启动命令（非阻塞）
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 gemini 命令失败: %w", err)
	}

	// 4. 使用 bufio.Scanner 实时读取输出
	// 使用 ScanRunes 可以确保每一个字符都能立即被推送到回调，提供最佳流式感
	scanner := bufio.NewScanner(stdout)
	scanner.Split(bufio.ScanRunes)

	// 在独立协程中执行读取，以防止阻塞主 select 逻辑
	scanDone := make(chan error, 1)
	go func() {
		for scanner.Scan() {
			onToken(scanner.Text())
		}
		scanDone <- scanner.Err()
	}()

	// 5. 等待执行结果或 Context 取消
	var finalErr error
	select {
	case <-ctx.Done():
		// Context 被取消或超时，exec.CommandContext 会自动杀掉进程
		finalErr = ctx.Err()
	case err := <-scanDone:
		if err != nil {
			finalErr = fmt.Errorf("读取 gemini 输出出错: %w", err)
		}
	}

	// 6. 资源清理与状态检查
	waitErr := cmd.Wait()
	if finalErr != nil {
		return finalErr
	}

	if waitErr != nil {
		// 如果是因为 Context 取消导致的 Wait 报错，优先返回 Context 错误
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("gemini 退出失败: %s", strings.TrimSpace(stderr.String()))
	}

	return nil
}

// 确保 Adapter 结构体实现了 cli_adapters.Adapter 接口。
var _ cli_adapters.Adapter = (*Adapter)(nil)
