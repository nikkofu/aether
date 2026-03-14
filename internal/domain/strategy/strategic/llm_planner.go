package strategic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nikkofu/aether/internal/domain/capability"
	"github.com/nikkofu/aether/internal/domain/knowledge"
	"github.com/nikkofu/aether/internal/domain/strategy/evolution"
	"github.com/nikkofu/aether/pkg/logging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// LLMStrategicPlanner 实现了基于动态模板和知识增强的战略规划。
type LLMStrategicPlanner struct {
	llm            capability.Capability
	graph          knowledge.Graph
	strategyEngine evolution.StrategyEngine // 注入策略进化引擎
	logger         logging.Logger
}

type goalPlanDraft struct {
	Title       string `json:"title"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Summary     string `json:"summary"`
}

type milestonePlanDraft struct {
	Title string `json:"title"`
	Name  string `json:"name"`
}

func NewLLMStrategicPlanner(llm capability.Capability, g knowledge.Graph, se evolution.StrategyEngine, log logging.Logger) *LLMStrategicPlanner {
	return &LLMStrategicPlanner{llm: llm, graph: g, strategyEngine: se, logger: log}
}

func (p *LLMStrategicPlanner) CreateVision(ctx context.Context, title, desc string) (*Vision, error) {
	v := &Vision{ID: uuid.New().String(), Title: title, Description: desc, CreatedAt: time.Now()}
	if p.graph != nil {
		p.graph.AddEntity(ctx, knowledge.Entity{
			ID: v.ID, Type: "vision", Name: v.Title,
			Metadata: map[string]any{"description": v.Description},
		}, "default")
	}
	return v, nil
}

func (p *LLMStrategicPlanner) PlanGoals(ctx context.Context, v Vision) ([]Goal, error) {
	// Tracing: strategic plan
	tracer := otel.Tracer("aether-tracer")
	var span oteltrace.Span
	ctx, span = tracer.Start(ctx, "strategic.plan.goals")
	span.SetAttributes(
		attribute.String("vision_id", v.ID),
		attribute.String("vision_title", v.Title),
	)
	defer span.End()

	orgID := "default" // 简化演示

	// 1. 获取当前活跃的策略模板 (不再硬编码 Prompt)
	template, err := p.strategyEngine.GetActive(ctx, orgID)
	if err != nil || template == nil {
		return nil, fmt.Errorf("未找到生效的战略模板: %w", err)
	}

	historyCtx := p.getHistoryContext(ctx, "goal")

	// 2. 为目标规划构建显式 Prompt，避免目标模板与里程碑模板隐式耦合。
	prompt := buildGoalPlanningPrompt(template.Content, v, historyCtx)

	content, err := p.callLLM(ctx, prompt)
	if err != nil {
		return nil, err
	}

	raw, err := parseGoalPlanDrafts(content)
	if err != nil {
		span.RecordError(err)
		if p.logger != nil {
			p.logger.Error(ctx, "LLM goal planning response parse failed",
				logging.Err(err),
				logging.String("raw_content", content),
				logging.String("vision_id", v.ID),
			)
		}
		return nil, fmt.Errorf("failed to parse goals JSON: %w", err)
	}

	goals := make([]Goal, 0, len(raw))
	for _, rg := range raw {
		title := firstNonEmptyPlannerValue(rg.Title, rg.Name, rg.Description, rg.Summary)
		description := firstNonEmptyPlannerValue(rg.Description, rg.Summary, title)
		if title == "" {
			continue
		}
		g := Goal{
			ID:          uuid.New().String(),
			VisionID:    v.ID,
			Title:       title,
			Description: description,
			Status:      "planned",
			CreatedAt:   time.Now(),
		}
		goals = append(goals, g)
		if p.graph != nil {
			p.graph.AddEntity(ctx, knowledge.Entity{ID: g.ID, Type: "goal", Name: g.Title}, orgID)
			p.graph.AddRelation(ctx, knowledge.Relation{ID: uuid.New().String(), FromID: v.ID, ToID: g.ID, Type: "has_goal"}, orgID)
		}
	}
	if len(goals) == 0 {
		return nil, fmt.Errorf("planner returned no goals for vision %s", v.ID)
	}
	return goals, nil
}

func (p *LLMStrategicPlanner) PlanMilestones(ctx context.Context, g Goal) ([]Milestone, error) {
	// Tracing: strategic plan
	tracer := otel.Tracer("aether-tracer")
	var span oteltrace.Span
	ctx, span = tracer.Start(ctx, "strategic.plan.milestones")
	span.SetAttributes(
		attribute.String("goal_id", g.ID),
		attribute.String("goal_title", g.Title),
	)
	defer span.End()

	orgID := "default"

	template, err := p.strategyEngine.GetActive(ctx, orgID)
	if err != nil || template == nil {
		return nil, fmt.Errorf("template not found")
	}

	historyCtx := p.getHistoryContext(ctx, "milestone")

	// 里程碑规划必须使用 goal 语义，不能再复用 vision-only 模板。
	prompt := buildMilestonePlanningPrompt(template.Content, g, historyCtx)

	content, err := p.callLLM(ctx, prompt)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// 记录 LLM 响应到 Span，方便调试
	span.SetAttributes(attribute.String("llm.response", content))

	raw, err := parseMilestonePlanDrafts(content)
	if err != nil {
		span.RecordError(err)
		if p.logger != nil {
			p.logger.Error(ctx, "LLM 响应 JSON 解析失败",
				logging.Err(err),
				logging.String("raw_content", content),
				logging.String("goal_id", g.ID),
			)
		}
		return nil, fmt.Errorf("failed to parse milestones JSON: %w", err)
	}

	milestones := make([]Milestone, 0, len(raw))
	for _, rm := range raw {
		title := firstNonEmptyPlannerValue(rm.Title, rm.Name)
		if title == "" {
			continue
		}
		m := Milestone{ID: uuid.New().String(), GoalID: g.ID, Title: title, Status: "pending", CreatedAt: time.Now()}
		milestones = append(milestones, m)
		if p.graph != nil {
			p.graph.AddEntity(ctx, knowledge.Entity{ID: m.ID, Type: "milestone", Name: m.Title}, orgID)
			p.graph.AddRelation(ctx, knowledge.Relation{ID: uuid.New().String(), FromID: g.ID, ToID: m.ID, Type: "has_milestone"}, orgID)
		}
	}
	if len(milestones) == 0 {
		return nil, fmt.Errorf("planner returned no milestones for goal %s", g.ID)
	}
	return milestones, nil
}

func (p *LLMStrategicPlanner) Replan(ctx context.Context, g Goal, feedback string) ([]Milestone, error) {
	return p.PlanMilestones(ctx, g)
}

func (p *LLMStrategicPlanner) getHistoryContext(ctx context.Context, entityType string) string {
	if p.graph == nil {
		return ""
	}
	var sb strings.Builder
	reflections, _ := p.graph.QueryByType(ctx, "default", "reflection")
	count := 0
	for _, ref := range reflections {
		if conf, ok := ref.Metadata["confidence"].(float64); ok && conf > 0.6 {
			sb.WriteString(fmt.Sprintf("- 经验记录: %s\n", ref.Name))
			count++
			if count >= 5 {
				break
			}
		}
	}
	return sb.String()
}

func (p *LLMStrategicPlanner) callLLM(ctx context.Context, prompt string) (string, error) {
	output, err := p.llm.Execute(ctx, map[string]any{
		"prompt":     prompt,
		"agent_name": "strategic_planner",
	})
	if err != nil {
		return "", err
	}
	c, _ := output["output"].(string)
	c = strings.TrimPrefix(c, "```json")
	c = strings.TrimSuffix(c, "```")
	return strings.TrimSpace(c), nil
}

func buildGoalPlanningPrompt(templateContent string, vision Vision, historyCtx string) string {
	fallback := fmt.Sprintf(
		"愿景: %s\n描述: %s\n历史: %s\n请生成 2-3 个战略目标，只输出 JSON 数组，每项包含 title 和 description 字段。",
		vision.Title,
		vision.Description,
		historyCtx,
	)

	if !strings.Contains(templateContent, "{{vision_title}}") && !strings.Contains(templateContent, "{{vision_desc}}") {
		return fallback
	}

	values := map[string]string{
		"history":      historyCtx,
		"vision_title": vision.Title,
		"vision_desc":  vision.Description,
		"goal_title":   "",
		"goal_desc":    "",
	}
	return fillPlanningTemplateOrFallback(templateContent, values, fallback)
}

func buildMilestonePlanningPrompt(templateContent string, goal Goal, historyCtx string) string {
	fallback := fmt.Sprintf(
		"目标: %s\n描述: %s\n历史: %s\n请生成 2-3 个可执行里程碑，只输出 JSON 数组，每项包含 title 字段。",
		goal.Title,
		goal.Description,
		historyCtx,
	)

	if !strings.Contains(templateContent, "{{goal_title}}") && !strings.Contains(templateContent, "{{goal_desc}}") {
		return fallback
	}

	values := map[string]string{
		"history":      historyCtx,
		"vision_title": "",
		"vision_desc":  "",
		"goal_title":   goal.Title,
		"goal_desc":    goal.Description,
	}
	return fillPlanningTemplateOrFallback(templateContent, values, fallback)
}

func fillPlanningTemplateOrFallback(templateContent string, values map[string]string, fallback string) string {
	prompt := templateContent
	for _, key := range []string{"history", "vision_title", "vision_desc", "goal_title", "goal_desc"} {
		prompt = strings.ReplaceAll(prompt, "{{"+key+"}}", values[key])
	}

	if strings.Contains(prompt, "{{") {
		return fallback
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return fallback
	}
	return prompt
}

func parseGoalPlanDrafts(content string) ([]goalPlanDraft, error) {
	normalized := normalizePlannerOutput(content)
	if normalized == "" {
		return nil, fmt.Errorf("empty planner output")
	}

	if drafts, ok := decodeGoalPlanDrafts(normalized); ok {
		return drafts, nil
	}

	if candidate := extractFirstJSONArray(normalized); candidate != "" && candidate != normalized {
		if drafts, ok := decodeGoalPlanDrafts(candidate); ok {
			return drafts, nil
		}
	}

	if drafts := parseGoalPlanDraftsFromLines(normalized); len(drafts) > 0 {
		return drafts, nil
	}

	return nil, fmt.Errorf("unrecognized planner output: %s", truncatePlannerOutput(normalized))
}

func parseMilestonePlanDrafts(content string) ([]milestonePlanDraft, error) {
	normalized := normalizePlannerOutput(content)
	if normalized == "" {
		return nil, fmt.Errorf("empty planner output")
	}

	if drafts, ok := decodeMilestonePlanDrafts(normalized); ok {
		return drafts, nil
	}

	if candidate := extractFirstJSONArray(normalized); candidate != "" && candidate != normalized {
		if drafts, ok := decodeMilestonePlanDrafts(candidate); ok {
			return drafts, nil
		}
	}

	if drafts := parseMilestonePlanDraftsFromLines(normalized); len(drafts) > 0 {
		return drafts, nil
	}

	return nil, fmt.Errorf("unrecognized planner output: %s", truncatePlannerOutput(normalized))
}

func normalizePlannerOutput(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(content), "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

func decodeGoalPlanDrafts(content string) ([]goalPlanDraft, bool) {
	var raw []goalPlanDraft
	if err := json.Unmarshal([]byte(content), &raw); err == nil {
		drafts := sanitizeGoalPlanDrafts(raw)
		if len(drafts) > 0 {
			return drafts, true
		}
	}

	var titles []string
	if err := json.Unmarshal([]byte(content), &titles); err == nil {
		drafts := make([]goalPlanDraft, 0, len(titles))
		for _, title := range titles {
			title = strings.TrimSpace(title)
			if title == "" {
				continue
			}
			drafts = append(drafts, goalPlanDraft{Title: title, Description: title})
		}
		if len(drafts) > 0 {
			return drafts, true
		}
	}

	return nil, false
}

func decodeMilestonePlanDrafts(content string) ([]milestonePlanDraft, bool) {
	var raw []milestonePlanDraft
	if err := json.Unmarshal([]byte(content), &raw); err == nil {
		drafts := sanitizeMilestonePlanDrafts(raw)
		if len(drafts) > 0 {
			return drafts, true
		}
	}

	var titles []string
	if err := json.Unmarshal([]byte(content), &titles); err == nil {
		drafts := make([]milestonePlanDraft, 0, len(titles))
		for _, title := range titles {
			title = strings.TrimSpace(title)
			if title == "" {
				continue
			}
			drafts = append(drafts, milestonePlanDraft{Title: title})
		}
		if len(drafts) > 0 {
			return drafts, true
		}
	}

	return nil, false
}

func sanitizeGoalPlanDrafts(raw []goalPlanDraft) []goalPlanDraft {
	drafts := make([]goalPlanDraft, 0, len(raw))
	for _, item := range raw {
		title := firstNonEmptyPlannerValue(item.Title, item.Name, item.Description, item.Summary)
		description := firstNonEmptyPlannerValue(item.Description, item.Summary, title)
		if title == "" {
			continue
		}
		drafts = append(drafts, goalPlanDraft{
			Title:       title,
			Description: description,
		})
	}
	return drafts
}

func sanitizeMilestonePlanDrafts(raw []milestonePlanDraft) []milestonePlanDraft {
	drafts := make([]milestonePlanDraft, 0, len(raw))
	for _, item := range raw {
		title := firstNonEmptyPlannerValue(item.Title, item.Name)
		if title == "" {
			continue
		}
		drafts = append(drafts, milestonePlanDraft{Title: title})
	}
	return drafts
}

func parseGoalPlanDraftsFromLines(content string) []goalPlanDraft {
	lines := plannerTextLines(content)
	drafts := make([]goalPlanDraft, 0, len(lines))
	for _, line := range lines {
		title, description := splitPlannerLine(line)
		title = strings.TrimSpace(title)
		description = firstNonEmptyPlannerValue(description, title)
		if title == "" {
			continue
		}
		drafts = append(drafts, goalPlanDraft{
			Title:       title,
			Description: description,
		})
	}
	return drafts
}

func parseMilestonePlanDraftsFromLines(content string) []milestonePlanDraft {
	lines := plannerTextLines(content)
	drafts := make([]milestonePlanDraft, 0, len(lines))
	for _, line := range lines {
		title, _ := splitPlannerLine(line)
		title = strings.TrimSpace(title)
		if title == "" {
			continue
		}
		drafts = append(drafts, milestonePlanDraft{Title: title})
	}
	return drafts
}

func plannerTextLines(content string) []string {
	lines := strings.Split(normalizePlannerOutput(content), "\n")
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		item := normalizePlannerLine(line)
		if item == "" {
			continue
		}
		items = append(items, item)
	}
	return items
}

func normalizePlannerLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}

	trimmed = strings.TrimLeft(trimmed, "-*• ")
	trimmed = strings.TrimLeft(trimmed, "0123456789.)、 ")
	trimmed = strings.TrimSpace(trimmed)
	trimmed = strings.Trim(trimmed, "\"'`")
	trimmed = strings.TrimSuffix(trimmed, ",")
	trimmed = strings.TrimSuffix(trimmed, ";")
	trimmed = strings.TrimSpace(trimmed)

	switch trimmed {
	case "", "[", "]", "{", "}":
		return ""
	}

	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "here are") || strings.HasPrefix(lower, "以下是") || strings.HasPrefix(lower, "说明") {
		return ""
	}

	return trimmed
}

func splitPlannerLine(line string) (string, string) {
	for _, separator := range []string{"：", ":"} {
		if idx := strings.Index(line, separator); idx > 0 {
			left := strings.TrimSpace(line[:idx])
			right := strings.TrimSpace(line[idx+len(separator):])
			left = strings.Trim(left, "\"'`")
			right = strings.Trim(right, "\"'`")
			if left != "" && right != "" {
				return left, right
			}
		}
	}

	for _, separator := range []string{" - ", " — ", " – "} {
		if idx := strings.Index(line, separator); idx > 0 {
			left := strings.TrimSpace(line[:idx])
			right := strings.TrimSpace(line[idx+len(separator):])
			left = strings.Trim(left, "\"'`")
			right = strings.Trim(right, "\"'`")
			if left != "" {
				return left, right
			}
		}
	}

	line = strings.Trim(line, "\"'`")
	return line, ""
}

func extractFirstJSONArray(content string) string {
	start := strings.Index(content, "[")
	if start < 0 {
		return ""
	}

	depth := 0
	inString := false
	escaped := false
	for index := start; index < len(content); index++ {
		ch := content[index]

		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return strings.TrimSpace(content[start : index+1])
			}
		}
	}

	return ""
}

func firstNonEmptyPlannerValue(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func truncatePlannerOutput(content string) string {
	content = strings.TrimSpace(content)
	if len(content) <= 160 {
		return content
	}
	return content[:160] + "..."
}
