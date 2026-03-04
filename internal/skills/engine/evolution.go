package engine

import (
	"context"
	"fmt"

	"github.com/nikkofu/aether/internal/audit"
	"github.com/nikkofu/aether/internal/logging"
	"github.com/nikkofu/aether/internal/policy"
	"github.com/nikkofu/aether/internal/skills"
	"github.com/nikkofu/aether/internal/skills/evaluator"
)

// SkillEvolutionEngine 负责技能的自动迭代、进化与淘汰。
type SkillEvolutionEngine struct {
	generator *SkillGenerator
	evaluator *evaluator.SkillEvaluator
	engine    skills.SkillEngine
	audit     audit.Logger
	logger    logging.Logger
	guard     *policy.EvolutionGuard // 更新引用
}

func NewSkillEvolutionEngine(g *SkillGenerator, ev *evaluator.SkillEvaluator, e skills.SkillEngine, a audit.Logger, l logging.Logger, guard *policy.EvolutionGuard) *SkillEvolutionEngine {
	return &SkillEvolutionEngine{
		generator: g,
		evaluator: ev,
		engine:    e,
		audit:     a,
		logger:    l,
		guard:     guard,
	}
}

// EvolveSkill 尝试对现有技能进行版本升级。
func (e *SkillEvolutionEngine) EvolveSkill(ctx context.Context, orgID, skillID string) error {
	// 1. 守卫校验
	if e.guard != nil && !e.guard.AllowEvolution("skills") {
		return fmt.Errorf("系统策略禁止技能进化")
	}

	activeSkills, err := e.engine.ListActive(ctx)
	if err != nil { return err }
	
	var skill *skills.Skill
	for i := range activeSkills {
		if activeSkills[i].ID == skillID {
			skill = &activeSkills[i]
			break
		}
	}
	if skill == nil { return fmt.Errorf("skill not found: %s", skillID) }

	versions, err := e.engine.ListVersions(ctx, skillID)
	if err != nil || len(versions) == 0 { return fmt.Errorf("no versions found") }
	
	var oldVersion *skills.SkillVersion
	for i := range versions {
		if versions[i].Active { oldVersion = &versions[i]; break }
	}
	if oldVersion == nil { oldVersion = &versions[0] }

	_, newV, err := e.generator.GenerateSkillFromSpec(ctx, orgID, skill.Description+" (请优化执行效率)")
	if err != nil { return err }

	newV.Parent = oldVersion.Version
	newScore, err := e.evaluator.EvaluateSkill(ctx, newV)
	if err != nil { return err }

	if newScore > oldVersion.Score {
		e.engine.RegisterVersion(ctx, *newV)
		return e.engine.ActivateVersion(ctx, skillID, newV.Version)
	}
	return nil
}

// PruneWeakSkills 自动清理低质量技能（末位淘汰制）。
func (e *SkillEvolutionEngine) PruneWeakSkills(ctx context.Context, orgID string, threshold float64) error {
	activeSkills, err := e.engine.ListActive(ctx)
	if err != nil { return err }

	for _, s := range activeSkills {
		versions, _ := e.engine.ListVersions(ctx, s.ID)
		if len(versions) == 0 { continue }
		
		currentV := versions[0] 
		if currentV.Score < threshold {
			e.logger.Warn(ctx, "技能评分过低，正在执行末位淘汰", 
				logging.String("skill", s.ID), 
				logging.Float64("score", currentV.Score))
			
			_ = e.engine.ActivateVersion(ctx, s.ID, "NONE") 
			
			if e.audit != nil {
				e.audit.Log(ctx, orgID, audit.EventReputationChange, "技能末位淘汰", map[string]any{"skill": s.ID, "score": currentV.Score})
			}
		}
	}
	return nil
}
