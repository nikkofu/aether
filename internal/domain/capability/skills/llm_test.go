package skills

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nikkofu/aether/internal/domain/capability"
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

func TestLLMSkill_Execute_Basic(t *testing.T) {
	mockAdapter := &MockAdapter{
		ExecuteFunc: func(ctx context.Context, prompt string) (string, error) {
			if !strings.Contains(prompt, "Hello Aether") {
				return "", errors.New("unexpected prompt")
			}
			return "Hello User", nil
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
		ExecuteFunc: func(ctx context.Context, prompt string) (string, error) {
			return prompt, nil // 直接返回渲染后的 prompt 以便验证
		},
	}

	skill := NewLLMSkill("test-skill", mockAdapter, nil, nil, nil, nil, nil, nil, "Default template")

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
			ExecuteFunc: func(ctx context.Context, prompt string) (string, error) {
				return "", errors.New("network error")
			},
		}

		skill := NewLLMSkill("error-skill", mockAdapter, nil, nil, nil, nil, nil, nil, "template")
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
		skill := NewLLMSkill("render-error", mockAdapter, nil, nil, nil, nil, nil, nil, "{{.unknown | invalid_func}}")
		
		_, err := skill.Execute(context.Background(), map[string]any{"prompt_data": map[string]any{}})
		if err == nil {
			t.Fatal("期望渲染失败，但实际成功")
		}
	})
}
