package org

import (
	"context"
	"testing"

	agentdomain "github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/internal/usecase/reflection"
)

func TestGovernanceAgentIgnoresNilReflection(t *testing.T) {
	agent := NewGovernanceAgent("governance")

	messages, err := agent.Handle(context.Background(), agentdomain.Message{
		ID:   "task-1",
		From: "operational-a",
		To:   agent.Name(),
		Type: "reflection.report",
		Payload: map[string]any{
			"reflection": (*reflection.Reflection)(nil),
		},
	})
	if err != nil {
		t.Fatalf("expected nil reflection payload to be ignored, got error: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected no governance messages for nil reflection, got %#v", messages)
	}
}
