package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nikkofu/aether/internal/domain/capability"
)

type promptCaptureCapability struct {
	lastPrompt string
	output     string
}

func (p *promptCaptureCapability) Name() string {
	return "prompt-capture"
}

func (p *promptCaptureCapability) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	p.lastPrompt, _ = input["prompt"].(string)
	return map[string]any{
		"output": p.output,
	}, nil
}

func TestCoderAgentInjectsDeliveryConstraintsIntoPrompt(t *testing.T) {
	llm := &promptCaptureCapability{output: "- Runtime path verified\n- Fix telemetry noise next"}
	agent := NewCoderAgent("coder", llm, nil)

	messages, err := agent.Handle(context.Background(), Message{
		ID:        "task-1",
		From:      "workflow.review_critique",
		To:        agent.Name(),
		Type:      "instruction",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"task":            "Write exactly 2 bullets for release readiness. Keep the total under 20 words.",
			"plan":            "Revise the draft and remove meta commentary.",
			"task_id":         "task-1",
			"delivery_target": "workflow.review_critique",
			"delivery_type":   TypeDraftGenerated,
			"progress_target": "supervisor",
		},
	})
	if err != nil {
		t.Fatalf("coder handling failed: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 response messages, got %d", len(messages))
	}

	if !strings.Contains(llm.lastPrompt, "Output only the final deliverable") {
		t.Fatalf("expected prompt to forbid meta commentary, got %q", llm.lastPrompt)
	}
	if !strings.Contains(llm.lastPrompt, "must output exactly 2 bullet points") {
		t.Fatalf("expected prompt to include extracted bullet constraint, got %q", llm.lastPrompt)
	}
	if !strings.Contains(llm.lastPrompt, "total word count must stay under 20") {
		t.Fatalf("expected prompt to include extracted word limit, got %q", llm.lastPrompt)
	}
}

func TestCoderAgentInjectsExactBulletSkeletonIntoPrompt(t *testing.T) {
	llm := &promptCaptureCapability{output: "- Ship Recommendation: public launch decision\n- Blockers: OpenTelemetry collector is not running\n- Next Action: fix the OpenTelemetry collector"}
	agent := NewCoderAgent("coder", llm, nil)

	_, err := agent.Handle(context.Background(), Message{
		ID:        "task-2",
		From:      "workflow.review_critique",
		To:        agent.Name(),
		Type:      "instruction",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"task":            "Write exactly 3 bullet points for the release memo. Use these exact bullet prefixes: '- Ship Recommendation:', '- Blockers:', and '- Next Action:'. The '- Ship Recommendation:' bullet must mention 'public launch decision'. The '- Blockers:' bullet must mention 'OpenTelemetry collector is not running'. The '- Next Action:' bullet must mention 'fix the OpenTelemetry collector'. Keep the total under 80 words.",
			"plan":            "Revise the draft using deterministic validation feedback.",
			"task_id":         "task-2",
			"delivery_target": "workflow.review_critique",
			"delivery_type":   TypeDraftGenerated,
			"progress_target": "supervisor",
		},
	})
	if err != nil {
		t.Fatalf("coder handling failed: %v", err)
	}

	if !strings.Contains(llm.lastPrompt, "Never place multiple required prefixes on the same line.") {
		t.Fatalf("expected prompt to forbid merged prefixes, got %q", llm.lastPrompt)
	}
	if !strings.Contains(llm.lastPrompt, "Line-by-line output skeleton:") {
		t.Fatalf("expected prompt to include line-by-line skeleton, got %q", llm.lastPrompt)
	}
	if !strings.Contains(llm.lastPrompt, "- Ship Recommendation: public launch decision") {
		t.Fatalf("expected prompt to include ship recommendation skeleton, got %q", llm.lastPrompt)
	}
	if !strings.Contains(llm.lastPrompt, "- Blockers: OpenTelemetry collector is not running") {
		t.Fatalf("expected prompt to include blockers skeleton, got %q", llm.lastPrompt)
	}
	if !strings.Contains(llm.lastPrompt, "- Next Action: fix the OpenTelemetry collector") {
		t.Fatalf("expected prompt to include next action skeleton, got %q", llm.lastPrompt)
	}
}

var _ capability.Capability = (*promptCaptureCapability)(nil)
