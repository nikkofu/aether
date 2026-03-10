package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	taskdomain "github.com/nikkofu/aether/internal/domain/task"
	taskusecase "github.com/nikkofu/aether/internal/usecase/task"
	"github.com/nikkofu/aether/pkg/logging"
)

type TaskSubmitter interface {
	Submit(ctx context.Context, input taskusecase.SubmitInput) (*taskdomain.Task, error)
}

// GitHubWebhookHandler 处理来自 GitHub 的事件。
type GitHubWebhookHandler struct {
	submitter TaskSubmitter
	logger    logging.Logger
}

// NewGitHubWebhookHandler 创建一个新的处理器实例。
func NewGitHubWebhookHandler(submitter TaskSubmitter, l logging.Logger) *GitHubWebhookHandler {
	return &GitHubWebhookHandler{
		submitter: submitter,
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

	// 我们目前只关注 issue 事件，以触发自动开发流程
	if event != "issues" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Event ignored"))
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

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
			Source:      "github_webhook",
			Mode:        "agent",
			Description: prompt,
			Input: map[string]any{
				"repository":  payload.Repository.FullName,
				"issue_title": payload.Issue.Title,
				"issue_body":  payload.Issue.Body,
				"issue_url":   payload.Issue.URL,
				"event":       event,
				"action":      payload.Action,
			},
		})
		if err != nil {
			http.Error(w, "Failed to create task", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "accepted",
			"task_id": task.ID,
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Accepted"))
}
