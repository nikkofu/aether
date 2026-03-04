package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	domain_skills "github.com/nikkofu/aether/internal/domain/capability/skills"
	"github.com/nikkofu/aether/pkg/logging"
)

// PolyglotCompiler 负责将任意主流编程语言的源代码通过 LLM 转化为符合 WASI 标准的 Go 代码，并自动编译为 WASM 模块。
type PolyglotCompiler struct {
	llm      domain_skills.SkillEngine
	llmSkill string
	logger   logging.Logger
	cacheDir string
}

func NewPolyglotCompiler(llm domain_skills.SkillEngine, llmSkillName string, logger logging.Logger, cacheDir string) *PolyglotCompiler {
	if cacheDir == "" {
		cacheDir = "/tmp/aether_build"
	}
	os.MkdirAll(cacheDir, 0755)
	return &PolyglotCompiler{
		llm:      llm,
		llmSkill: llmSkillName,
		logger:   logger,
		cacheDir: cacheDir,
	}
}

// Compile 接收原始代码及其语言类型，返回编译后的 WASM 绝对路径。
func (c *PolyglotCompiler) Compile(ctx context.Context, sourceCode, sourceLang, skillName string) (string, error) {
	var targetGoCode string

	// 1. 语言转换阶段 (如果是 Go，则跳过)
	if sourceLang == "go" {
		targetGoCode = sourceCode
	} else {
		if c.logger != nil {
			c.logger.Info(ctx, "正在使用 LLM 将代码转换为 WASI Go", logging.String("from", sourceLang))
		}
		
		prompt := fmt.Sprintf("将以下 %s 代码重写为符合 wazero 和 WASI snapshot_preview1 标准的 Go 代码。\n"+
			"要求：\n"+
			"1. 必须使用 package main 和 func main()。\n"+
			"2. 通过 os.Stdin 读取 JSON 输入，通过 os.Stdout 输出 JSON 结果。\n"+
			"3. 只能使用 Go 标准库，不引入第三方依赖。\n"+
			"4. 仅返回纯代码文本，不要包含任何 Markdown 代码块包裹（如 ```go）。\n\n"+
			"源代码：\n%s", sourceLang, sourceCode)

		input := map[string]any{
			"prompt": prompt,
			"agent_name": "compiler_agent",
		}

		result, err := c.llm.Execute(ctx, c.llmSkill, input)
		if err != nil {
			return "", fmt.Errorf("LLM 翻译代码失败: %w", err)
		}

		targetGoCode, _ = result["output"].(string)
	}

	// 2. 准备编译工作区
	buildDir := filepath.Join(c.cacheDir, fmt.Sprintf("build_%d", time.Now().UnixNano()))
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return "", err
	}
	defer os.RemoveAll(buildDir) // 编译完成后清理源码

	mainFile := filepath.Join(buildDir, "main.go")
	if err := os.WriteFile(mainFile, []byte(targetGoCode), 0644); err != nil {
		return "", err
	}

	// 3. 执行系统级编译 (GOOS=wasip1 GOARCH=wasm)
	targetWasmFile := filepath.Join(c.cacheDir, fmt.Sprintf("%s_%d.wasm", skillName, time.Now().Unix()))
	
	cmd := exec.CommandContext(ctx, "go", "build", "-o", targetWasmFile, "main.go")
	cmd.Dir = buildDir
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		if c.logger != nil {
			c.logger.Error(ctx, "WASM 编译失败", logging.String("output", string(output)))
		}
		return "", fmt.Errorf("WASM 编译失败: %s", string(output))
	}

	// 4. 验证生成的 WASM 文件
	if stat, err := os.Stat(targetWasmFile); err != nil || stat.Size() == 0 {
		return "", fmt.Errorf("生成的 WASM 文件无效或为空")
	}

	if c.logger != nil {
		c.logger.Info(ctx, "WASM 动态编译成功", logging.String("path", targetWasmFile))
	}

	return targetWasmFile, nil
}
