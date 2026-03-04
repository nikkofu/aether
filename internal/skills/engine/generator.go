package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nikkofu/aether/internal/audit"
	"github.com/nikkofu/aether/internal/capability"
	"github.com/nikkofu/aether/internal/skills"
	"github.com/nikkofu/aether/internal/skills/security"
)

// SkillGenerator 负责从规格描述动态生成并编译 WASM 技能。
type SkillGenerator struct {
	llm       capability.Capability
	validator *security.SkillValidator
	audit     audit.Logger
}

func NewSkillGenerator(llm capability.Capability, a audit.Logger) *SkillGenerator {
	return &SkillGenerator{
		llm:       llm,
		validator: security.NewSkillValidator(),
		audit:     a,
	}
}

// GenerateSkillFromSpec 生成 TinyGo 代码并执行带内存限制 (64MB) 的 WASM 编译。
func (g *SkillGenerator) GenerateSkillFromSpec(ctx context.Context, orgID, spec string) (*skills.Skill, *skills.SkillVersion, error) {
	// 1. 调用 LLM 生成 TinyGo 兼容代码
	prompt := fmt.Sprintf(`你是一个高级 TinyGo 开发专家。请根据以下需求编写一个独立的 WASM 插件源码。
需求: %s

【开发约束】：
1. 必须是单文件 main 包。
2. 必须实现从 os.Stdin 读取 JSON 并输出结果到 os.Stdout。
3. 目标环境为 WASI，仅限标准库。

请直接输出代码内容：`, spec)

	output, err := g.llm.Execute(ctx, map[string]any{"prompt": prompt})
	if err != nil { return nil, nil, err }
	code, _ := output["output"].(string)
	code = g.cleanCode(code)

	// 2. 安全检查
	if err := g.validator.ValidateCode(code); err != nil {
		if g.audit != nil { g.audit.Log(ctx, orgID, audit.EventConstRejected, "技能代码安全性拦截", map[string]any{"err": err.Error()}) }
		return nil, nil, err
	}

	// 3. TinyGo 编译：强制限制 1024 个 64KB Page (= 64MB)
	skillID := uuid.New().String()
	tmpDir := filepath.Join(os.TempDir(), "aether-wasm", skillID)
	os.MkdirAll(tmpDir, 0755)
	srcPath := filepath.Join(tmpDir, "main.go")
	wasmPath := filepath.Join(tmpDir, "skill.wasm")
	
	if err := os.WriteFile(srcPath, []byte(code), 0644); err != nil { return nil, nil, err }

	buildCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// 编译铁律：-target=wasi 确保可移植性，-max-pages=1024 确保物理隔离
	cmd := exec.CommandContext(buildCtx, "tinygo", "build", "-o", wasmPath, "-target=wasi", "-max-pages=1024", srcPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, nil, fmt.Errorf("TinyGo 编译失败: %s", string(out))
	}

	// 4. 构造对象
	skill := &skills.Skill{ID: skillID, Name: "Wasm-" + skillID[:8], Description: spec, Active: true, CreatedAt: time.Now()}
	version := &skills.SkillVersion{SkillID: skillID, Version: "v1.0.0", CodePath: wasmPath, Active: true, CreatedAt: time.Now()}

	if g.audit != nil {
		g.audit.Log(ctx, orgID, audit.EventProposalPassed, "WASM 技能编译成功并应用内存限制", map[string]any{"id": skillID, "memory_limit": "64MB"})
	}

	return skill, version, nil
}

func (g *SkillGenerator) cleanCode(code string) string {
	code = strings.TrimPrefix(code, "```go")
	code = strings.TrimPrefix(code, "```")
	code = strings.TrimSuffix(code, "```")
	return strings.TrimSpace(code)
}
