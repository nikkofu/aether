package ollama

import "time"

// Config 定义了 Ollama 适配器的配置项。
type Config struct {
	BaseURL     string
	Model       string
	Temperature float64
	Timeout     time.Duration
}
