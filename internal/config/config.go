package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 汇总了 Aether 系统所有的配置项。
type Config struct {
	App struct {
		Mode   string `mapstructure:"mode"`    // single, cluster-leader, cluster-worker
		NodeID string `mapstructure:"node_id"` // 当前节点的唯一标识
		Role   string `mapstructure:"role"`    // 仅用于 worker 模式
	} `mapstructure:"app"`

	Agent struct {
		MaxSpawnPerTask int `mapstructure:"max_spawn_per_task"`
		MaxConcurrency  int `mapstructure:"max_concurrency"`
	} `mapstructure:"agent"`

	Runtime struct {
		GeminiCommand string `mapstructure:"gemini_command"`
		DatabasePath  string `mapstructure:"database_path"`
		NATSURL       string `mapstructure:"nats_url"`
	} `mapstructure:"runtime"`

	OpenAI struct {
		BaseURL     string        `mapstructure:"base_url"`
		APIKey      string        `mapstructure:"api_key"`
		Model       string        `mapstructure:"model"`
		Temperature float64       `mapstructure:"temperature"`
		Timeout     time.Duration `mapstructure:"timeout"`
	} `mapstructure:"openai"`

	Log struct {
		Level string `mapstructure:"level"`
	} `mapstructure:"log"`
}

// Load 从指定路径加载配置文件，并合并环境变量和默认值。
func Load(path string) (*Config, error) {
	v := viper.New()

	// 1. 设置默认值
	v.SetDefault("app.mode", "single")
	v.SetDefault("app.node_id", "default-node")
	v.SetDefault("agent.max_spawn_per_task", 5)
	v.SetDefault("agent.max_concurrency", 20)
	v.SetDefault("runtime.gemini_command", "gemini")
	v.SetDefault("runtime.database_path", "./aether.db")
	v.SetDefault("openai.base_url", "https://api.openai.com/v1")
	v.SetDefault("openai.model", "gpt-4o")
	v.SetDefault("openai.temperature", 0.7)
	v.SetDefault("openai.timeout", 120*time.Second)
	v.SetDefault("log.level", "info")

	// 2. 配置文件支持
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./configs")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("读取配置文件失败: %w", err)
		}
	}

	// 3. 环境变量支持
	v.SetEnvPrefix("AETHER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 4. 解析到结构体
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	return &cfg, nil
}
