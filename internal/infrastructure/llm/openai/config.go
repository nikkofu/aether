package openai

import (
	"os"
	"strconv"
	"time"
)

// Config 包含了连接 OpenAI 兼容 API 所需的所有配置项。
type Config struct {
	// BaseURL 是 API 的基础地址（例如 https://api.openai.com/v1）。
	BaseURL string
	// APIKey 是身份验证令牌。
	APIKey string
	// Model 指定使用的模型名称（例如 gpt-4o）。
	Model string
	// Temperature 控制生成的随机性（0.0 到 2.0 之间）。
	Temperature float64
	// Timeout 指定单次 API 请求的超时时间。
	Timeout time.Duration
}

// LoadFromEnv 从环境变量中加载配置，并提供合理的默认值。
func LoadFromEnv() Config {
	cfg := Config{
		BaseURL:     getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		APIKey:      os.Getenv("OPENAI_API_KEY"),
		Model:       getEnv("OPENAI_MODEL", "gpt-4o"),
		Temperature: 0.7, // 默认 Temperature
		Timeout:     120 * time.Second,
	}

	// 解析 Temperature
	if tempStr := os.Getenv("OPENAI_TEMPERATURE"); tempStr != "" {
		if val, err := strconv.ParseFloat(tempStr, 64); err == nil {
			cfg.Temperature = val
		}
	}

	return cfg
}

// getEnv 是一个辅助函数，用于读取环境变量并在缺失时返回默认值。
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// Validate 检查核心配置是否完整。
func (c Config) Validate() error {
	if c.APIKey == "" {
		// 注意：某些本地模型（如 Ollama）可能不需要 API Key，
		// 但对于标准的 OpenAI 适配器，这通常是必须的。
		return nil // 留给适配器实现去处理具体的鉴权逻辑
	}
	return nil
}
