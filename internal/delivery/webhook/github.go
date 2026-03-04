package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/pkg/bus"
	"github.com/nikkofu/aether/pkg/logging"
)

// GitHubWebhookHandler 处理来自 GitHub 的事件。
type GitHubWebhookHandler struct {
	bus    bus.Bus
	logger logging.Logger
}

// NewGitHubWebhookHandler 创建一个新的处理器实例。
func NewGitHubWebhookHandler(b bus.Bus, l logging.Logger) *GitHubWebhookHandler {
	return &GitHubWebhookHandler{
		bus:    b,
		logger: l,
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

		// 发布系统消息，要求生成一个 Planner 代理来分析和处理该 Issue
		taskID := "task-" + uuid.New().String()[:8]
		prompt := fmt.Sprintf("Analyze and resolve the following GitHub Issue in %s:\nTitle: %s\n\nBody:\n%s\nURL: %s",
			payload.Repository.FullName, payload.Issue.Title, payload.Issue.Body, payload.Issue.URL)

		h.bus.Publish(context.Background(), agent.Message{
			ID:        uuid.New().String(),
			From:      "webhook_gateway",
			To:        "manager", // 发送给 Manager 请求调度
			Type:      "system.spawn",
			Timestamp: time.Now(),
			Payload: map[string]any{
				"role": "planner",
				"payload": map[string]any{
					"task_id": taskID,
					"prompt":  prompt,
				},
			},
		})
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Accepted"))
}
