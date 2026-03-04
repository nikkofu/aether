package engine

import (
	"context"
	"fmt"
	"time"

	domain_skills "github.com/nikkofu/aether/internal/domain/capability/skills"
	"github.com/nikkofu/aether/internal/domain/knowledge"
	"github.com/nikkofu/aether/internal/domain/policy"
	"github.com/nikkofu/aether/pkg/logging"
)

// StrategyEngine 定义了自进化策略的核心引擎。
type StrategyEngine interface {
	// Evolve 根据当前运行时的反馈对技能进行演进优化。
	Evolve(ctx context.Context, skillID string) (*domain_skills.SkillVersion, error)
}

// DefaultStrategyEngine 实现了基于 LLM 反思与知识图谱的自动策略演进。
type DefaultStrategyEngine struct {
	llmSkill       domain_skills.Skill
	graph          knowledge.Graph
	logger         logging.Logger
	evolutionGuard *policy.EvolutionGuard
}

func NewDefaultStrategyEngine(llm domain_skills.Skill, g knowledge.Graph, l logging.Logger, eg *policy.EvolutionGuard) *DefaultStrategyEngine {
	return &DefaultStrategyEngine{
		llmSkill:       llm,
		graph:          g,
		logger:         l,
		evolutionGuard: eg,
	}
}

func (e *DefaultStrategyEngine) Evolve(ctx context.Context, skillID string) (*domain_skills.SkillVersion, error) {
	// 演进逻辑：
	// 1. 获取最近的执行失败或性能指标
	// 2. 调用 LLMSkill 进行反思并生成优化建议
	// 3. 生成新的 SkillVersion 代码或配置
	
	// 模拟演进成功
	v := &domain_skills.SkillVersion{
		SkillID:    skillID,
		Version:    fmt.Sprintf("v%d", time.Now().Unix()),
		CodePath:   "/tmp/evolved.wasm",
		EntryPoint: "main",
		CreatedAt:  time.Now(),
	}

	if e.evolutionGuard != nil {
		if !e.evolutionGuard.AllowEvolution("skills") {
			return nil, fmt.Errorf("演进被政策拦截: 模块 skills 不允许自主演进")
		}
	}

	return v, nil
}
