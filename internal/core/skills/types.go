package skills

import (
	"context"
	"time"
)

// Skill 定义了 Aether 系统的动态能力单元（插件/工具）的逻辑标识。
type Skill struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedBy   string    `json:"created_by"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
}

// SkillVersion 代表技能的一个具体版本，支持树状演进结构。
type SkillVersion struct {
	SkillID     string    `json:"skill_id"`
	Version     string    `json:"version"`
	// Parent 指向该版本所继承或优化的父版本号。
	Parent      string    `json:"parent,omitempty"`
	// CodePath 指向该版本执行文件的路径。
	CodePath    string    `json:"code_path"`
	// EntryPoint 是该版本的执行入口。
	EntryPoint  string    `json:"entry_point"`
	// Score 代表该版本的质量评分。
	Score       float64   `json:"score"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
}

// SkillEngine 负责管理技能及其版本的注册、激活、演进及动态执行。
type SkillEngine interface {
	// Register 向引擎中注册一个新技能。
	Register(ctx context.Context, skill Skill) error
	// RegisterVersion 注册技能的一个新版本。
	RegisterVersion(ctx context.Context, version SkillVersion) error
	// ActivateVersion 启用技能的特定版本。
	ActivateVersion(ctx context.Context, skillID, version string) error
	// ListActive 列出当前所有已激活的技能（及其当前生效版本）。
	ListActive(ctx context.Context) ([]Skill, error)
	// GetVersion 获取技能的具体版本详情。
	GetVersion(ctx context.Context, skillID, version string) (*SkillVersion, error)
	// ListVersions 获取技能的所有演进版本。
	ListVersions(ctx context.Context, skillID string) ([]SkillVersion, error)
	// Execute 动态执行指定技能（通常执行其当前的 Active 版本）。
	Execute(ctx context.Context, skillID string, input map[string]any) (map[string]any, error)
}
