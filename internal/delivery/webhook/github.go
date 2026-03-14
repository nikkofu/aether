package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"strings"
	"sync"

	taskdomain "github.com/nikkofu/aether/internal/domain/task"
	taskusecase "github.com/nikkofu/aether/internal/usecase/task"
	"github.com/nikkofu/aether/pkg/logging"
)

type TaskSubmitter interface {
	Submit(ctx context.Context, input taskusecase.SubmitInput) (*taskdomain.Task, error)
}

// GitHubWebhookHandler 处理来自 GitHub 的事件。
type GitHubWebhookHandler struct {
	submitter    TaskSubmitter
	store        DeliveryStore
	secret       string
	logger       logging.Logger
	warnNoSecret sync.Once
}

// NewGitHubWebhookHandler 创建一个新的处理器实例。
func NewGitHubWebhookHandler(submitter TaskSubmitter, store DeliveryStore, secret string, l logging.Logger) *GitHubWebhookHandler {
	return &GitHubWebhookHandler{
		submitter: submitter,
		store:     store,
		secret:    secret,
		logger:    l,
	}
}

// Handle 处理 HTTP 请求。
func (h *GitHubWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	event := r.Header.Get("X-GitHub-Event")
	if event == "" {
		http.Error(w, "Missing X-GitHub-Event header", http.StatusBadRequest)
		return
	}

	deliveryID := r.Header.Get("X-GitHub-Delivery")
	if deliveryID == "" {
		http.Error(w, "Missing X-GitHub-Delivery header", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	if !h.verifySignature(body, r.Header.Get("X-Hub-Signature-256")) {
		http.Error(w, "Invalid webhook signature", http.StatusUnauthorized)
		return
	}

	payloadHash := hashPayload(body)
	record, shouldProcess, err := h.acquireDelivery(r.Context(), deliveryID, event, payloadHash)
	if err != nil {
		http.Error(w, "Failed to acquire delivery", http.StatusConflict)
		return
	}
	if !shouldProcess {
		h.respondDuplicate(w, record)
		return
	}

	// 我们目前只关注 issue 事件，以触发自动开发流程
	if event != "issues" {
		h.completeDelivery(r.Context(), deliveryID, DeliveryStatusIgnored, "", "")
		h.writeJSON(w, http.StatusOK, map[string]string{
			"status":      "ignored",
			"delivery_id": deliveryID,
		})
		return
	}

	var payload struct {
		Action string `json:"action"`
		Issue  struct {
			Title string `json:"title"`
			Body  string `json:"body"`
			URL   string `json:"html_url"`
		} `json:"issue"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		h.completeDelivery(r.Context(), deliveryID, DeliveryStatusFailed, "", err.Error())
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// 仅在新建 issue 时触发 Agent
	if payload.Action == "opened" {
		if h.logger != nil {
			h.logger.Info(context.Background(), "Received new GitHub Issue",
				logging.String("repo", payload.Repository.FullName),
				logging.String("title", payload.Issue.Title),
			)
		}

		prompt := fmt.Sprintf("Analyze and resolve the following GitHub Issue in %s:\nTitle: %s\n\nBody:\n%s\nURL: %s",
			payload.Repository.FullName, payload.Issue.Title, payload.Issue.Body, payload.Issue.URL)

		task, err := h.submitter.Submit(r.Context(), taskusecase.SubmitInput{
			Source:          "github_webhook",
			Mode:            "agent",
			WorkflowPattern: taskdomain.PatternSequential,
			Description:     prompt,
			Input: map[string]any{
				"delivery_id": deliveryID,
				"repository":  payload.Repository.FullName,
				"issue_title": payload.Issue.Title,
				"issue_body":  payload.Issue.Body,
				"issue_url":   payload.Issue.URL,
				"event":       event,
				"action":      payload.Action,
			},
		})
		if err != nil {
			h.completeDelivery(r.Context(), deliveryID, DeliveryStatusFailed, "", err.Error())
			http.Error(w, "Failed to create task", http.StatusInternalServerError)
			return
		}

		h.completeDelivery(r.Context(), deliveryID, DeliveryStatusAccepted, task.ID, "")
		h.writeJSON(w, http.StatusAccepted, map[string]string{
			"status":      "accepted",
			"task_id":     task.ID,
			"delivery_id": deliveryID,
		})
		return
	}

	h.completeDelivery(r.Context(), deliveryID, DeliveryStatusIgnored, "", "")
	h.writeJSON(w, http.StatusOK, map[string]string{
		"status":      "ignored",
		"delivery_id": deliveryID,
	})
}

func (h *GitHubWebhookHandler) acquireDelivery(ctx context.Context, deliveryID, event, payloadHash string) (*DeliveryRecord, bool, error) {
	if h.store == nil {
		return &DeliveryRecord{
			DeliveryID:    deliveryID,
			Event:         event,
			PayloadSHA256: payloadHash,
			Status:        DeliveryStatusPending,
		}, true, nil
	}

	return h.store.Acquire(ctx, DeliveryRecord{
		DeliveryID:    deliveryID,
		Event:         event,
		PayloadSHA256: payloadHash,
	})
}

func (h *GitHubWebhookHandler) completeDelivery(ctx context.Context, deliveryID, status, taskID, errorMessage string) {
	if h.store == nil {
		return
	}
	if err := h.store.Complete(ctx, deliveryID, status, taskID, errorMessage); err != nil && h.logger != nil {
		h.logger.Error(ctx, "failed to update webhook delivery state",
			logging.String("delivery_id", deliveryID),
			logging.String("status", status),
			logging.Err(err),
		)
	}
}

func (h *GitHubWebhookHandler) respondDuplicate(w http.ResponseWriter, record *DeliveryRecord) {
	if record == nil {
		h.writeJSON(w, http.StatusAccepted, map[string]string{
			"status": "duplicate",
		})
		return
	}

	switch record.Status {
	case DeliveryStatusAccepted:
		h.writeJSON(w, http.StatusAccepted, map[string]string{
			"status":      "duplicate",
			"task_id":     record.TaskID,
			"delivery_id": record.DeliveryID,
		})
	case DeliveryStatusIgnored:
		h.writeJSON(w, http.StatusOK, map[string]string{
			"status":      "ignored_duplicate",
			"delivery_id": record.DeliveryID,
		})
	default:
		h.writeJSON(w, http.StatusAccepted, map[string]string{
			"status":      "in_progress",
			"delivery_id": record.DeliveryID,
		})
	}
}

func (h *GitHubWebhookHandler) verifySignature(body []byte, signature string) bool {
	if h.secret == "" {
		h.warnNoSecret.Do(func() {
			if h.logger != nil {
				h.logger.Warn(context.Background(), "GitHub webhook secret is not configured; signature verification is disabled")
			}
		})
		return true
	}

	signature = strings.TrimSpace(signature)
	if signature == "" || !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	expected := signatureForPayload([]byte(h.secret), body)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func signatureForPayload(secret, body []byte) string {
	var mac hash.Hash = hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func hashPayload(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (h *GitHubWebhookHandler) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
