package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nikkofu/aether/internal/core/capability"
)

// SkillHandler 处理所有与技能相关的命令行指令。
type SkillHandler struct {
	registry *capability.CapabilityRegistry
}

// NewSkillHandler 创建一个新的 SkillHandler。
func NewSkillHandler(reg *capability.CapabilityRegistry) *SkillHandler {
	return &SkillHandler{
		registry: reg,
	}
}

// Handle 执行 skill 子命令的分发。
func (h *SkillHandler) Handle(ctx context.Context, args []string) {
	if len(args) < 1 {
		h.printUsage()
		os.Exit(1)
	}

	subCommand := args[0]
	subArgs := args[1:]

	switch subCommand {
	case "list":
		h.listSkills()
	case "run":
		h.runSkill(ctx, subArgs)
	default:
		fmt.Fprintf(os.Stderr, "错误: 未知 skill 子命令 '%s'\n", subCommand)
		h.printUsage()
		os.Exit(1)
	}
}

// listSkills 列出所有已注册的技能。
func (h *SkillHandler) listSkills() {
	names := h.registry.ListNames()
	h.printJSON(map[string]any{
		"skills": names,
		"total":  len(names),
	})
}

// runSkill 执行指定的技能，支持流式输出。
func (h *SkillHandler) runSkill(ctx context.Context, args []string) {
	runCmd := flag.NewFlagSet("skill run", flag.ExitOnError)
	inputJSON := runCmd.String("input", "{}", "技能输入的 JSON 字符串")
	stream := runCmd.Bool("stream", false, "是否启用流式输出")
	
	// 在 Parse 之前处理 args，确保 flag 能够被正确解析，无论位置如何
	// 但最标准的做法是让用户遵循 [flags] [args] 顺序
	runCmd.Parse(args)

	remaining := runCmd.Args()
	if len(remaining) < 1 {
		fmt.Fprintln(os.Stderr, "错误: 必须指定技能名称")
		fmt.Fprintln(os.Stderr, "用法: aether skill run [flags] <skill_name>")
		runCmd.Usage()
		os.Exit(1)
	}
	skillName := remaining[0]

	skill, ok := h.registry.Get(skillName)
	if !ok {
		h.printErrorJSON(fmt.Errorf("未找到技能: %s", skillName))
		os.Exit(1)
	}

	var input map[string]any
	if err := json.Unmarshal([]byte(*inputJSON), &input); err != nil {
		h.printErrorJSON(fmt.Errorf("无效的输入 JSON: %w", err))
		os.Exit(1)
	}

	if *stream {
		input["stream"] = true
	}

	start := time.Now()
	output, err := skill.Execute(ctx, input)
	duration := time.Since(start)

	if err != nil {
		h.printErrorJSON(err)
		os.Exit(1)
	}

	if *stream {
		fmt.Fprintf(os.Stderr, "\n[执行完成] 耗时: %s\n", duration.Round(time.Millisecond))
	} else {
		h.printJSON(map[string]any{
			"skill":    skillName,
			"result":   output,
			"status":   "success",
			"duration": duration.String(),
		})
	}
}

func (h *SkillHandler) printJSON(data any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 编码失败: %v\n", err)
	}
}

func (h *SkillHandler) printErrorJSON(err error) {
	h.printJSON(map[string]any{
		"status": "error",
		"error":  err.Error(),
	})
}

func (h *SkillHandler) printUsage() {
	fmt.Println("用法: aether skill <command> [arguments]")
	fmt.Println("\n可用命令:")
	fmt.Println("  list            列出所有可用技能")
	fmt.Println("  run [flags] <name> 执行指定技能")
	fmt.Println("\n'run' 命令参数:")
	fmt.Println("  --input string  传递给技能的 JSON 输入 (默认 \"{}\")")
	fmt.Println("  --stream        启用流式输出实时打印内容")
}
