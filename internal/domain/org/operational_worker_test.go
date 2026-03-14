package org

import (
	"strings"
	"testing"
)

func TestBuildOperationalTaskPromptDemandsFinalDeliverable(t *testing.T) {
	prompt := buildOperationalTaskPrompt("Write exactly 3 bullet points with the prefixes '- Decision:', '- Evidence:', and '- Next Step:'.")
	if !strings.Contains(prompt, "return only the final deliverable") {
		t.Fatalf("expected deliverable-oriented worker prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "exact-prefix requirements literally") {
		t.Fatalf("expected explicit contract guidance, got %q", prompt)
	}
}

func TestBuildOperationalTaskPromptUsesChineseVariantWhenNeeded(t *testing.T) {
	prompt := buildOperationalTaskPrompt("输出 3 条发布前检查要点")
	if !strings.Contains(prompt, "你是执行型代理") {
		t.Fatalf("expected Chinese worker prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "最终可交付结果") {
		t.Fatalf("expected Chinese deliverable wording, got %q", prompt)
	}
}
