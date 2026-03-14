package skills

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nikkofu/aether/internal/domain/capability"
	"github.com/nikkofu/aether/internal/domain/strategy"
	"github.com/nikkofu/aether/internal/infrastructure/llm"
)

// MockAdapter 用于测试的适配器 Mock
type MockAdapter struct {
	NameFunc    func() string
	ExecuteFunc func(ctx context.Context, prompt string) (string, error)
	StreamFunc  func(ctx context.Context, prompt string, onToken llm.TokenCallback) error
}

func (m *MockAdapter) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return "mock"
}

func (m *MockAdapter) Execute(ctx context.Context, prompt string) (string, error) {
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx, prompt)
	}
	return "mock response", nil
}

func (m *MockAdapter) Stream(ctx context.Context, prompt string, onToken llm.TokenCallback) error {
	if m.StreamFunc != nil {
		return m.StreamFunc(ctx, prompt, onToken)
	}
	return nil
}

type stubStrategyStore struct {
	strategy *strategy.Strategy
}

func (s *stubStrategyStore) Get(agentName string) (*strategy.Strategy, error) {
	if s.strategy == nil || s.strategy.AgentName != agentName {
		return nil, errors.New("strategy not found")
	}
	return s.strategy, nil
}

func (s *stubStrategyStore) Save(st *strategy.Strategy) error {
	s.strategy = st
	return nil
}

func TestLLMSkill_Execute_Basic(t *testing.T) {
	mockAdapter := &MockAdapter{
		StreamFunc: func(ctx context.Context, prompt string, onToken llm.TokenCallback) error {
			if !strings.Contains(prompt, "Hello Aether") {
				return errors.New("unexpected prompt")
			}
			onToken("Hello User")
			return nil
		},
	}

	skill := NewLLMSkill(
		"test-skill",
		mockAdapter,
		nil, // provider
		nil, // router
		nil, // tracker
		nil, // tracer
		nil, // strategyStore
		capability.NewDefaultRenderer(),
		"Greet: {{.prompt_data.name}}",
		nil, // bus
	)

	input := map[string]any{
		"prompt_data": map[string]any{
			"name": "Hello Aether",
		},
	}

	result, err := skill.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}

	if result["output"] != "Hello User" {
		t.Errorf("期望 output 为 'Hello User', 实际得到: %v", result["output"])
	}
	if result["status"] != "success" {
		t.Errorf("期望 status 为 'success', 实际得到: %v", result["status"])
	}
}

func TestLLMSkill_Execute_CustomTemplate(t *testing.T) {
	mockAdapter := &MockAdapter{
		StreamFunc: func(ctx context.Context, prompt string, onToken llm.TokenCallback) error {
			onToken(prompt)
			return nil // 直接返回渲染后的 prompt 以便验证
		},
	}

	skill := NewLLMSkill("test-skill", mockAdapter, nil, nil, nil, nil, nil, nil, "Default template", nil)

	input := map[string]any{
		"prompt": "Custom: {{.prompt_data.val}}",
		"prompt_data": map[string]any{
			"val": "override",
		},
	}

	result, err := skill.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}

	if result["output"] != "Custom: override" {
		t.Errorf("自定义模板渲染失败, 得到: %v", result["output"])
	}
}

func TestLLMSkill_Execute_ErrorCase(t *testing.T) {
	t.Run("AdapterError", func(t *testing.T) {
		mockAdapter := &MockAdapter{
			StreamFunc: func(ctx context.Context, prompt string, onToken llm.TokenCallback) error {
				return errors.New("network error")
			},
		}

		skill := NewLLMSkill("error-skill", mockAdapter, nil, nil, nil, nil, nil, nil, "template", nil)
		_, err := skill.Execute(context.Background(), map[string]any{"prompt_data": map[string]any{}})

		if err == nil {
			t.Fatal("期望得到错误，但实际没有")
		}

		if !strings.Contains(err.Error(), "network error") {
			t.Errorf("期望错误包含 'network error', 实际得到: %v", err)
		}
	})

	t.Run("RenderingError", func(t *testing.T) {
		mockAdapter := &MockAdapter{}
		// 使用一个会触发渲染错误的模板 (引用不存在的函数)
		skill := NewLLMSkill("render-error", mockAdapter, nil, nil, nil, nil, nil, nil, "{{.unknown | invalid_func}}", nil)

		_, err := skill.Execute(context.Background(), map[string]any{"prompt_data": map[string]any{}})
		if err == nil {
			t.Fatal("期望渲染失败，但实际成功")
		}
	})
}

func TestLLMSkillInjectsLearnedOperatingRules(t *testing.T) {
	var capturedPrompt string
	mockAdapter := &MockAdapter{
		StreamFunc: func(ctx context.Context, prompt string, onToken llm.TokenCallback) error {
			capturedPrompt = prompt
			onToken("ok")
			return nil
		},
	}

	strategyStore := &stubStrategyStore{
		strategy: &strategy.Strategy{
			AgentName:  "coder",
			PromptHint: "Satisfy exact bullet prefixes in order before improving wording. | Return only the final deliverable with no preface, explanation, or meta commentary.",
			RetryLimit: 1,
			UpdatedAt:  time.Now(),
		},
	}

	skill := NewLLMSkill("test-skill", mockAdapter, nil, nil, nil, nil, strategyStore, nil, "Default template", nil)
	_, err := skill.Execute(context.Background(), map[string]any{
		"agent_name": "coder",
		"prompt":     "Write the deliverable.",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(capturedPrompt, "Learned operating rules from previous task reflections:") {
		t.Fatalf("expected learned rules prelude, got %q", capturedPrompt)
	}
	if !strings.Contains(capturedPrompt, "- Satisfy exact bullet prefixes in order before improving wording.") {
		t.Fatalf("expected first learned rule bullet, got %q", capturedPrompt)
	}
	if !strings.Contains(capturedPrompt, "Apply these learned rules unless the current task explicitly conflicts with them.") {
		t.Fatalf("expected learned rules footer, got %q", capturedPrompt)
	}
}
