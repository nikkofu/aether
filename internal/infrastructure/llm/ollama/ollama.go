package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nikkofu/aether/internal/infrastructure/llm"
)

// Adapter 实现了与本地 Ollama 服务的交互。
type Adapter struct {
	config Config
	client *http.Client
}

// NewOllamaAdapter 创建一个新的 Ollama 适配器实例。
func NewOllamaAdapter(cfg Config) *Adapter {
	return &Adapter{
		config: cfg,
		// 关键修复：不再设置全局 Timeout，否则流式传输会因为 Body 读取时间长而被 client 强制杀掉
		client: &http.Client{
			Transport: &http.Transport{
				ResponseHeaderTimeout: cfg.Timeout, // 仅限制响应头超时
			},
		},
	}
}

// Name 返回适配器的名称标识。
func (a *Adapter) Name() string {
	return "ollama"
}

// Execute 发送请求并阻塞等待完整结果。
func (a *Adapter) Execute(ctx context.Context, prompt string) (string, error) {
	reqBody := map[string]any{
		"model":  a.config.Model,
		"prompt": prompt,
		"stream": false,
		"options": map[string]any{
			"temperature": a.config.Temperature,
		},
	}
	
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/generate", a.config.BaseURL), bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama API 错误: %s", resp.Status)
	}

	var resData struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resData); err != nil {
		return "", err
	}

	return resData.Response, nil
}

// Stream 发送请求并流式返回结果。
func (a *Adapter) Stream(ctx context.Context, prompt string, onToken llm.TokenCallback) error {
	reqBody := map[string]any{
		"model":  a.config.Model,
		"prompt": prompt,
		"stream": true,
		"options": map[string]any{"temperature": a.config.Temperature},
	}
	
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/generate", a.config.BaseURL), bytes.NewBuffer(body))
	if err != nil { return err }
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama API 错误: %s", resp.Status)
	}

	// 核心修复：改用 Decoder 逐个解析 JSON 对象，不再依赖换行符
	decoder := json.NewDecoder(resp.Body)
	for {
		var chunk struct {
			Response string `json:"response"`
			Done     bool   `json:"done"`
		}
		if err := decoder.Decode(&chunk); err != nil {
			if err.Error() == "EOF" { break }
			return err
		}

		if chunk.Response != "" && onToken != nil {
			onToken(chunk.Response)
		}
		if chunk.Done { break }
	}
	return nil
}

var _ llm.Adapter = (*Adapter)(nil)
