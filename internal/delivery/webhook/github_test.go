package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	count     int
	lastInput taskusecase.SubmitInput
	err       error
}

func (f *fakeSubmitter) Submit(ctx context.Context, input taskusecase.SubmitInput) (*taskdomain.Task, error) {
	f.count++
	f.lastInput = input
	if f.err != nil {
		return nil, f.err
	}

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
	handler := NewGitHubWebhookHandler(submitter, NewInMemoryDeliveryStore(), "topsecret", &webhookLogger{})

	payload := testIssuePayload()
	req := newSignedGitHubRequest(t, "delivery-1", "issues", payload, "topsecret")
	w := httptest.NewRecorder()

	handler.Handle(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	if submitter.count != 1 {
		t.Fatalf("expected submit count 1, got %d", submitter.count)
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
	if deliveryID, _ := submitter.lastInput.Input["delivery_id"].(string); deliveryID != "delivery-1" {
		t.Fatalf("expected delivery_id=delivery-1, got %v", submitter.lastInput.Input["delivery_id"])
	}
}

func TestGitHubWebhookHandler_RejectsInvalidSignature(t *testing.T) {
	submitter := &fakeSubmitter{}
	handler := NewGitHubWebhookHandler(submitter, NewInMemoryDeliveryStore(), "topsecret", &webhookLogger{})

	payload := testIssuePayload()
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBuffer(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "delivery-invalid-signature")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")

	w := httptest.NewRecorder()
	handler.Handle(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if submitter.count != 0 {
		t.Fatalf("expected submit count 0, got %d", submitter.count)
	}
}

func TestGitHubWebhookHandler_DeduplicatesAcceptedDelivery(t *testing.T) {
	submitter := &fakeSubmitter{}
	handler := NewGitHubWebhookHandler(submitter, NewInMemoryDeliveryStore(), "topsecret", &webhookLogger{})

	payload := testIssuePayload()

	firstReq := newSignedGitHubRequest(t, "delivery-duplicate", "issues", payload, "topsecret")
	firstRes := httptest.NewRecorder()
	handler.Handle(firstRes, firstReq)

	secondReq := newSignedGitHubRequest(t, "delivery-duplicate", "issues", payload, "topsecret")
	secondRes := httptest.NewRecorder()
	handler.Handle(secondRes, secondReq)

	if submitter.count != 1 {
		t.Fatalf("expected submit count 1 after duplicate delivery, got %d", submitter.count)
	}
	if secondRes.Code != http.StatusAccepted {
		t.Fatalf("expected duplicate response status 202, got %d", secondRes.Code)
	}

	var payloadResp map[string]string
	if err := json.NewDecoder(secondRes.Body).Decode(&payloadResp); err != nil {
		t.Fatalf("failed to decode duplicate response: %v", err)
	}
	if payloadResp["status"] != "duplicate" {
		t.Fatalf("expected duplicate status, got %s", payloadResp["status"])
	}
	if payloadResp["task_id"] != "task-123" {
		t.Fatalf("expected duplicate task_id task-123, got %s", payloadResp["task_id"])
	}
}

func TestGitHubWebhookHandler_RetriesFailedDelivery(t *testing.T) {
	submitter := &fakeSubmitter{err: errors.New("submit failed")}
	handler := NewGitHubWebhookHandler(submitter, NewInMemoryDeliveryStore(), "topsecret", &webhookLogger{})

	payload := testIssuePayload()

	firstReq := newSignedGitHubRequest(t, "delivery-retry", "issues", payload, "topsecret")
	firstRes := httptest.NewRecorder()
	handler.Handle(firstRes, firstReq)

	if firstRes.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on first attempt, got %d", firstRes.Code)
	}
	if submitter.count != 1 {
		t.Fatalf("expected submit count 1 after first failure, got %d", submitter.count)
	}

	submitter.err = nil

	secondReq := newSignedGitHubRequest(t, "delivery-retry", "issues", payload, "topsecret")
	secondRes := httptest.NewRecorder()
	handler.Handle(secondRes, secondReq)

	if secondRes.Code != http.StatusAccepted {
		t.Fatalf("expected 202 on retry, got %d", secondRes.Code)
	}
	if submitter.count != 2 {
		t.Fatalf("expected submit count 2 after retry, got %d", submitter.count)
	}
}

func TestGitHubWebhookHandler_IgnoresNonIssueEvents(t *testing.T) {
	submitter := &fakeSubmitter{}
	handler := NewGitHubWebhookHandler(submitter, NewInMemoryDeliveryStore(), "topsecret", &webhookLogger{})

	payload := map[string]any{"action": "created"}
	req := newSignedGitHubRequest(t, "delivery-ping", "ping", payload, "topsecret")
	w := httptest.NewRecorder()

	handler.Handle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for ignored event, got %d", w.Code)
	}
	if submitter.count != 0 {
		t.Fatalf("expected submit count 0, got %d", submitter.count)
	}
}

func newSignedGitHubRequest(t *testing.T, deliveryID, event string, payload map[string]any, secret string) *http.Request {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBuffer(body))
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("X-GitHub-Delivery", deliveryID)
	req.Header.Set("X-Hub-Signature-256", signatureForPayload([]byte(secret), body))
	return req
}

func testIssuePayload() map[string]any {
	return map[string]any{
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
}
