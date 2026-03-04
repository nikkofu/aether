package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/nikkofu/aether/internal/infrastructure/llm"
	"github.com/nikkofu/aether/pkg/observability"
	"github.com/nikkofu/aether/pkg/observability/trace"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Usage 包含了单次请求的 Token 消耗信息。
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// OpenAIAdapter 实现了对 OpenAI 兼容 API 的调用。
type OpenAIAdapter struct {
	cfg         Config
	client      *http.Client
	tracer      observability.Tracer
	traceEngine *trace.TraceEngine
}

func NewOpenAIAdapter(cfg Config) *OpenAIAdapter {
	return &OpenAIAdapter{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// SetTracer 允许动态注入追踪器。
func (a *OpenAIAdapter) SetTracer(t observability.Tracer) {
	a.tracer = t
}

func (a *OpenAIAdapter) SetTraceEngine(te *trace.TraceEngine) {
	a.traceEngine = te
}

func (a *OpenAIAdapter) Name() string {
	return "openai"
}

func (a *OpenAIAdapter) Execute(ctx context.Context, prompt string) (string, error) {
	content, _, err := a.ExecuteWithUsage(ctx, prompt)
	return content, err
}

func (a *OpenAIAdapter) Stream(ctx context.Context, prompt string, onToken llm.TokenCallback) error {
	return a.StreamWithUsage(ctx, prompt, onToken, nil)
}

func (a *OpenAIAdapter) ExecuteWithUsage(ctx context.Context, prompt string) (string, Usage, error) {
	// Tracing: external API call
	if a.traceEngine != nil {
		var span oteltrace.Span
		ctx, span = a.traceEngine.StartSpan(ctx, "external API call: "+a.cfg.Model)
		span.SetAttributes(
			attribute.String("model", a.cfg.Model),
			attribute.String("adapter", "openai"),
		)
		defer span.End()
	}

	reqBody, _ := json.Marshal(ChatRequest{
		Model:       a.cfg.Model,
		Temperature: a.cfg.Temperature,
		Messages:    []ChatMessage{{Role: "user", Content: prompt}},
	})

	req, _ := http.NewRequestWithContext(ctx, "POST", a.cfg.BaseURL+"/chat/completions", bytes.NewReader(reqBody))
	a.setHeaders(req)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", Usage{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", Usage{}, fmt.Errorf("API error: %s", string(body))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", Usage{}, err
	}

	if len(chatResp.Choices) == 0 {
		return "", Usage{}, fmt.Errorf("empty choices")
	}

	return chatResp.Choices[0].Message.Content, Usage{
		PromptTokens:     chatResp.Usage.PromptTokens,
		CompletionTokens: chatResp.Usage.CompletionTokens,
		TotalTokens:      chatResp.Usage.TotalTokens,
	}, nil
}

func (a *OpenAIAdapter) StreamWithUsage(ctx context.Context, prompt string, onToken llm.TokenCallback, onUsage func(Usage)) error {
	if a.tracer != nil {
		var span observability.Span
		ctx, span = a.tracer.StartSpan(ctx, "OpenAI.Stream", map[string]any{"model": a.cfg.Model})
		defer span.End()
	}

	reqBody, _ := json.Marshal(ChatRequest{
		Model:       a.cfg.Model,
		Temperature: a.cfg.Temperature,
		Stream:      true,
		StreamOptions: &struct {
			IncludeUsage bool `json:"include_usage"`
		}{IncludeUsage: true},
		Messages: []ChatMessage{{Role: "user", Content: prompt}},
	})

	req, _ := http.NewRequestWithContext(ctx, "POST", a.cfg.BaseURL+"/chat/completions", bytes.NewReader(reqBody))
	a.setHeaders(req)

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) > 0 {
			onToken(chunk.Choices[0].Delta.Content)
		}
		if chunk.Usage != nil && onUsage != nil {
			onUsage(Usage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			})
		}
	}
	return scanner.Err()
}

func (a *OpenAIAdapter) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if a.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.cfg.APIKey)
	}
}

type ChatRequest struct {
	Model         string        `json:"model"`
	Messages      []ChatMessage `json:"messages"`
	Temperature   float64       `json:"temperature"`
	Stream        bool          `json:"stream"`
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

var _ llm.Adapter = (*OpenAIAdapter)(nil)
