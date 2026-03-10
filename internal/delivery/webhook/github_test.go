package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	taskdomain "github.com/nikkofu/aether/internal/domain/task"
	taskusecase "github.com/nikkofu/aether/internal/usecase/task"
	"github.com/nikkofu/aether/pkg/logging"
)

type webhookLogger struct{}

func (w *webhookLogger) Debug(ctx context.Context, msg string, fields ...logging.Field) {}
func (w *webhookLogger) Info(ctx context.Context, msg string, fields ...logging.Field)  {}
func (w *webhookLogger) Warn(ctx context.Context, msg string, fields ...logging.Field)  {}
func (w *webhookLogger) Error(ctx context.Context, msg string, fields ...logging.Field) {}
func (w *webhookLogger) Sync() error                                                    { return nil }

type fakeSubmitter struct {
	lastInput taskusecase.SubmitInput
}

func (f *fakeSubmitter) Submit(ctx context.Context, input taskusecase.SubmitInput) (*taskdomain.Task, error) {
	f.lastInput = input
	return &taskdomain.Task{
		ID:          "task-123",
		Source:      input.Source,
		Mode:        input.Mode,
		Description: input.Description,
		Status:      taskdomain.StatusRunning,
	}, nil
}

func TestGitHubWebhookHandler_CreatesTask(t *testing.T) {
	submitter := &fakeSubmitter{}
	handler := NewGitHubWebhookHandler(submitter, &webhookLogger{})

	payload := map[string]any{
		"action": "opened",
		"issue": map[string]any{
			"title":    "Task intake is broken",
			"body":     "Please route webhook events through the task service.",
			"html_url": "https://github.com/nikkofu/aether/issues/1",
		},
		"repository": map[string]any{
			"full_name": "nikkofu/aether",
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBuffer(body))
	req.Header.Set("X-GitHub-Event", "issues")
	w := httptest.NewRecorder()

	handler.Handle(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	if submitter.lastInput.Source != "github_webhook" {
		t.Fatalf("expected github_webhook source, got %s", submitter.lastInput.Source)
	}
	if submitter.lastInput.Mode != "agent" {
		t.Fatalf("expected agent mode, got %s", submitter.lastInput.Mode)
	}
	if submitter.lastInput.Description == "" {
		t.Fatal("expected a generated task description")
	}
}
