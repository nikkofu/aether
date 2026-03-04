package reflection

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nikkofu/aether/internal/domain/capability"
)

// LLMReflector 使用 LLM 技能进行执行质量分析。
type LLMReflector struct {
	llm capability.Capability
}

// NewLLMReflector 创建一个新的 LLM 反思器。
func NewLLMReflector(llm capability.Capability) *LLMReflector {
	return &LLMReflector{llm: llm}
}

// Reflect 执行反思逻辑。
func (r *LLMReflector) Reflect(ctx context.Context, input ReflectionInput) (*Reflection, error) {
	errStr := "None"
	if input.Error != nil {
		errStr = input.Error.Error()
	}

	// 1. 构造反思 Prompt
	prompt := fmt.Sprintf(`你是企业级AI质量分析系统。
分析以下执行情况并给出专业评估：
- Agent: %s
- Success: %v
- Duration: %s
- Cost: $%f
- Error: %s

请严格按以下格式输出：
[ANALYSIS]
此处填写执行质量评估和失败原因分析（如有）。
[SUGGESTIONS]
- 建议1
- 建议2`, input.AgentName, input.Error == nil, input.Duration, input.Cost, errStr)

	// 2. 调用 LLM
	output, err := r.llm.Execute(ctx, map[string]any{"prompt": prompt})
	if err != nil {
		return nil, fmt.Errorf("反思调用失败: %w", err)
	}

	content, _ := output["output"].(string)

	// 3. 解析输出
	analysis, suggestions := r.parseOutput(content)

	// 4. 返回结构化结果
	return &Reflection{
		ID:              uuid.New().String(),
		AgentName:       input.AgentName,
		TaskID:          input.TaskID,
		Success:         input.Error == nil,
		Duration:        input.Duration,
		Cost:            input.Cost,
		ErrorMessage:    errStr,
		Analysis:        analysis,
		Suggestions:     suggestions,
		ConfidenceScore: 0.5,
		CreatedAt:       time.Now(),
	}, nil
}

func (r *LLMReflector) parseOutput(content string) (string, []string) {
	var analysis string
	var suggestions []string

	parts := strings.Split(content, "[SUGGESTIONS]")
	if len(parts) > 0 {
		analysis = strings.TrimPrefix(parts[0], "[ANALYSIS]")
		analysis = strings.TrimSpace(analysis)
	}

	if len(parts) > 1 {
		lines := strings.Split(parts[1], "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") {
				suggestions = append(suggestions, strings.TrimSpace(line[1:]))
			} else {
				suggestions = append(suggestions, line)
			}
		}
	}

	return analysis, suggestions
}
