package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nikkofu/aether/internal/domain/capability"
)

type stubCapability struct {
	name   string
	output string
	err    error
	calls  int
}

func (s *stubCapability) Name() string {
	if s.name != "" {
		return s.name
	}
	return "stub"
}

func (s *stubCapability) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return map[string]any{
		"output": s.output,
	}, nil
}

func TestReviewerAgentApprovesCompliantDeliverable(t *testing.T) {
	reviewer := NewReviewerAgent("reviewer", &stubCapability{
		output: "Thought: The deliverable satisfies the task.\nDecision: [PASS]\nFeedback: Looks good.",
	}, nil)

	messages, err := reviewer.Handle(context.Background(), Message{
		ID:        "task-1",
		From:      "workflow.review_critique",
		To:        reviewer.Name(),
		Type:      "review_request",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"task_id": "task-1",
			"task":    "Write exactly 2 bullets for release readiness. Keep the total under 20 words.",
			"code":    "- Runtime path verified\n- Fix telemetry noise next",
		},
	})
	if err != nil {
		t.Fatalf("review handling failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 response, got %d", len(messages))
	}

	approved, _ := messages[0].Payload["approved"].(bool)
	if !approved {
		t.Fatalf("expected review approval, got %#v", messages[0].Payload)
	}

	if source, _ := messages[0].Payload["review_decision_source"].(string); source != "deterministic_contract_pass" {
		t.Fatalf("expected deterministic contract pass source, got %q", source)
	}
	if reviewer.llmSkill.(*stubCapability).calls != 0 {
		t.Fatalf("expected deterministic pass to skip llm review, got %d calls", reviewer.llmSkill.(*stubCapability).calls)
	}
}

func TestReviewerAgentRejectsConstraintViolationsEvenWhenLLMPasses(t *testing.T) {
	reviewer := NewReviewerAgent("reviewer", &stubCapability{
		output: "Thought: This is acceptable.\nDecision: [PASS]\nFeedback: Ship it.",
	}, nil)

	messages, err := reviewer.Handle(context.Background(), Message{
		ID:        "task-2",
		From:      "workflow.review_critique",
		To:        reviewer.Name(),
		Type:      "review_request",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"task_id": "task-2",
			"task":    "Write exactly 2 bullets for release readiness. Keep the total under 12 words.",
			"code":    "I will prepare the release checklist.\n- Runtime path verified\n- Fix telemetry noise next",
		},
	})
	if err != nil {
		t.Fatalf("review handling failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 response, got %d", len(messages))
	}

	approved, _ := messages[0].Payload["approved"].(bool)
	if approved {
		t.Fatalf("expected deterministic validator to reject output, got %#v", messages[0].Payload)
	}

	feedback, _ := messages[0].Payload["feedback"].(string)
	if feedback == "" || !containsAll(feedback, "Deterministic Validation", "bullet-only output") {
		t.Fatalf("expected merged validation feedback, got %q", feedback)
	}

	violations, _ := messages[0].Payload["quality_gate_violations"].([]string)
	if len(violations) == 0 {
		t.Fatalf("expected deliverable quality violations, got %#v", messages[0].Payload["quality_gate_violations"])
	}

	if source, _ := messages[0].Payload["review_decision_source"].(string); source != "deterministic_contract_fail" {
		t.Fatalf("expected deterministic contract fail source, got %q", source)
	}
}

func TestReviewerAgentRejectsAmbiguousReviewWithoutDecision(t *testing.T) {
	reviewer := NewReviewerAgent("reviewer", &stubCapability{
		output: "Thought: Looks mostly fine.\nFeedback: Tighten the wording a little.",
	}, nil)

	messages, err := reviewer.Handle(context.Background(), Message{
		ID:        "task-3",
		From:      "workflow.review_critique",
		To:        reviewer.Name(),
		Type:      "review_request",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"task_id": "task-3",
			"task":    "Summarize the release state.",
			"code":    "Release path verified.",
		},
	})
	if err != nil {
		t.Fatalf("review handling failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 response, got %d", len(messages))
	}

	approved, _ := messages[0].Payload["approved"].(bool)
	if approved {
		t.Fatalf("expected ambiguous review to be rejected, got %#v", messages[0].Payload)
	}

	protocolViolations, _ := messages[0].Payload["reviewer_protocol_violations"].([]string)
	if len(protocolViolations) == 0 {
		t.Fatalf("expected reviewer protocol violations, got %#v", messages[0].Payload["reviewer_protocol_violations"])
	}
}

func TestReviewerAgentApprovesContractCompliantDeliverableWhenDecisionLineIsMissing(t *testing.T) {
	reviewer := NewReviewerAgent("reviewer", &stubCapability{
		output: "Thought: The deliverable is concise and follows the required shape.\nFeedback: Ready to ship.",
	}, nil)

	messages, err := reviewer.Handle(context.Background(), Message{
		ID:        "task-4",
		From:      "workflow.review_critique",
		To:        reviewer.Name(),
		Type:      "review_request",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"task_id": "task-4",
			"task":    "Write exactly 2 bullets for release readiness. Use these exact bullet prefixes: '- Ship Recommendation:' and '- Next Action:'. Keep the total under 20 words.",
			"code":    "- Ship Recommendation: Ship now\n- Next Action: Monitor telemetry",
		},
	})
	if err != nil {
		t.Fatalf("review handling failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 response, got %d", len(messages))
	}

	approved, _ := messages[0].Payload["approved"].(bool)
	if !approved {
		t.Fatalf("expected contract fallback approval, got %#v", messages[0].Payload)
	}

	source, _ := messages[0].Payload["review_decision_source"].(string)
	if source != "deterministic_contract_pass" {
		t.Fatalf("expected deterministic_contract_pass decision source, got %q", source)
	}

	if reviewer.llmSkill.(*stubCapability).calls != 0 {
		t.Fatalf("expected deterministic contract pass to skip llm review, got %d calls", reviewer.llmSkill.(*stubCapability).calls)
	}
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}

var _ capability.Capability = (*stubCapability)(nil)
