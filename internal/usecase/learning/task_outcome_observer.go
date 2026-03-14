package learning

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nikkofu/aether/internal/domain/knowledge"
	taskdomain "github.com/nikkofu/aether/internal/domain/task"
	"github.com/nikkofu/aether/internal/usecase/reflection"
	"github.com/nikkofu/aether/pkg/logging"
)

// TaskOutcomeObserver converts terminal task outcomes into deterministic reflections
// so evaluator-optimizer style workflows can adapt without requiring an extra LLM pass.
type TaskOutcomeObserver struct {
	reflectionStore reflection.Store
	learningEngine  *LearningEngine
	graph           knowledge.Graph
	logger          logging.Logger
}

func NewTaskOutcomeObserver(store reflection.Store, engine *LearningEngine, graph knowledge.Graph, logger logging.Logger) *TaskOutcomeObserver {
	return &TaskOutcomeObserver{
		reflectionStore: store,
		learningEngine:  engine,
		graph:           graph,
		logger:          logger,
	}
}

func (o *TaskOutcomeObserver) ObserveTerminalTask(ctx context.Context, task *taskdomain.Task, events []*taskdomain.Event) error {
	if o == nil || task == nil {
		return nil
	}
	if task.Status != taskdomain.StatusCompleted && task.Status != taskdomain.StatusFailed {
		return nil
	}
	if len(events) == 0 {
		return nil
	}

	reflections := buildTaskOutcomeReflections(task, events)
	if len(reflections) == 0 {
		return nil
	}

	orgID := taskOrgID(task, events)
	for _, item := range reflections {
		if item == nil {
			continue
		}
		if o.reflectionStore != nil {
			if err := o.reflectionStore.Save(ctx, item); err != nil {
				return err
			}
		}
		if o.learningEngine != nil {
			if err := o.learningEngine.UpdateStrategy(item); err != nil {
				return err
			}
		}
		if o.graph != nil {
			_ = o.graph.AddEntity(ctx, knowledge.Entity{
				ID:   item.ID,
				Type: "reflection",
				Name: fmt.Sprintf("Task Reflection: %s", item.AgentName),
				Metadata: map[string]any{
					"agent_name":       item.AgentName,
					"task_id":          item.TaskID,
					"success":          item.Success,
					"confidence_score": item.ConfidenceScore,
				},
				CreatedAt: item.CreatedAt,
			}, orgID)
			_ = o.graph.AddRelation(ctx, knowledge.Relation{
				ID:        "rel-" + item.ID,
				FromID:    task.ID,
				ToID:      item.ID,
				Type:      "reflected",
				CreatedAt: item.CreatedAt,
			}, orgID)
		}
		if o.logger != nil {
			o.logger.Info(ctx, "task outcome reflection captured",
				logging.String("task_id", task.ID),
				logging.String("agent_name", item.AgentName),
				logging.Any("success", item.Success),
			)
		}
	}

	return nil
}

type taskOutcomeSignals struct {
	patternLabel                string
	reviewResults               int
	reviewFailures              int
	maxIteration                int
	qualityViolations           []string
	protocolViolations          []string
	decisionSourceCounts        map[string]int
	coderTouched                bool
	reviewerTouched             bool
	plannerFailedPlanValidation bool
}

func buildTaskOutcomeReflections(task *taskdomain.Task, events []*taskdomain.Event) []*reflection.Reflection {
	signals := summarizeTaskOutcome(task, events)
	reflections := make([]*reflection.Reflection, 0, 3)

	if coderReflection := buildCoderReflection(task, signals); coderReflection != nil {
		reflections = append(reflections, coderReflection)
	}
	if reviewerReflection := buildReviewerReflection(task, signals); reviewerReflection != nil {
		reflections = append(reflections, reviewerReflection)
	}
	if plannerReflection := buildPlannerReflection(task, signals); plannerReflection != nil {
		reflections = append(reflections, plannerReflection)
	}

	return reflections
}

func summarizeTaskOutcome(task *taskdomain.Task, events []*taskdomain.Event) taskOutcomeSignals {
	signals := taskOutcomeSignals{
		patternLabel:         workflowPatternLabel(task.WorkflowPattern),
		decisionSourceCounts: make(map[string]int),
	}

	for _, event := range events {
		if event == nil {
			continue
		}

		if roleMatches(event.From, "coder") || roleMatches(event.To, "coder") {
			signals.coderTouched = true
		}
		if roleMatches(event.From, "reviewer") || roleMatches(event.To, "reviewer") {
			signals.reviewerTouched = true
		}

		switch event.Type {
		case "review_result":
			signals.reviewerTouched = true
			signals.reviewResults++
			if approved, ok := boolValue(event.Payload["approved"]); ok && !approved {
				signals.reviewFailures++
			}
			signals.qualityViolations = appendUniqueStrings(signals.qualityViolations, stringSlice(event.Payload["quality_gate_violations"])...)
			signals.protocolViolations = appendUniqueStrings(signals.protocolViolations, stringSlice(event.Payload["reviewer_protocol_violations"])...)
			if source := strings.TrimSpace(stringValue(event.Payload["review_decision_source"])); source != "" {
				signals.decisionSourceCounts[source]++
			}
		case "review_request", "draft.generated", "instruction":
			if iteration := intValue(event.Payload["iteration"]); iteration > signals.maxIteration {
				signals.maxIteration = iteration
			}
		case "system.alert":
			message := strings.ToLower(strings.TrimSpace(stringValue(event.Payload["message"])))
			if strings.Contains(message, "valid plan string") {
				signals.plannerFailedPlanValidation = true
			}
		}
	}

	if strings.Contains(strings.ToLower(task.ErrorSummary), "valid plan string") {
		signals.plannerFailedPlanValidation = true
	}

	return signals
}

func buildCoderReflection(task *taskdomain.Task, signals taskOutcomeSignals) *reflection.Reflection {
	if !signals.coderTouched && signals.reviewResults == 0 && task.WorkflowPattern != taskdomain.PatternReviewCritique && task.WorkflowPattern != taskdomain.PatternIterativeRefinement && task.WorkflowPattern != taskdomain.PatternLoop {
		return nil
	}

	needsLearning := task.Status == taskdomain.StatusFailed || signals.reviewFailures > 0 || len(signals.qualityViolations) > 0
	if !needsLearning {
		return nil
	}

	suggestions := buildCoderSuggestions(task, signals)
	return &reflection.Reflection{
		ID:              uuid.New().String(),
		AgentName:       "coder",
		TaskID:          task.ID,
		Success:         false,
		Duration:        0,
		Cost:            0,
		ErrorMessage:    strings.TrimSpace(task.ErrorSummary),
		Analysis:        buildCoderAnalysis(task, signals),
		Suggestions:     suggestions,
		ConfidenceScore: 0.9,
		CreatedAt:       time.Now(),
	}
}

func buildReviewerReflection(task *taskdomain.Task, signals taskOutcomeSignals) *reflection.Reflection {
	if !signals.reviewerTouched {
		return nil
	}

	nonLLMDecisions := signals.decisionSourceCounts["repair"] + signals.decisionSourceCounts["contract_fallback"] + signals.decisionSourceCounts["heuristic"] + signals.decisionSourceCounts["missing"]
	needsLearning := len(signals.protocolViolations) > 0 || nonLLMDecisions > 0
	if !needsLearning {
		return nil
	}

	suggestions := buildReviewerSuggestions(signals)
	return &reflection.Reflection{
		ID:              uuid.New().String(),
		AgentName:       "reviewer",
		TaskID:          task.ID,
		Success:         false,
		Duration:        0,
		Cost:            0,
		ErrorMessage:    strings.TrimSpace(task.ErrorSummary),
		Analysis:        buildReviewerAnalysis(task, signals, nonLLMDecisions),
		Suggestions:     suggestions,
		ConfidenceScore: 0.95,
		CreatedAt:       time.Now(),
	}
}

func buildPlannerReflection(task *taskdomain.Task, signals taskOutcomeSignals) *reflection.Reflection {
	if task.WorkflowPattern != taskdomain.PatternSequential || !signals.plannerFailedPlanValidation {
		return nil
	}

	return &reflection.Reflection{
		ID:           uuid.New().String(),
		AgentName:    "planner",
		TaskID:       task.ID,
		Success:      false,
		Duration:     0,
		Cost:         0,
		ErrorMessage: strings.TrimSpace(task.ErrorSummary),
		Analysis:     fmt.Sprintf("The sequential prompt-chaining workflow failed because the planner did not return a valid plan string. Terminal status: %s.", task.Status),
		Suggestions: []string{
			"Return a plain implementation plan string immediately instead of meta commentary or invalid wrapper text.",
		},
		ConfidenceScore: 0.85,
		CreatedAt:       time.Now(),
	}
}

func buildCoderAnalysis(task *taskdomain.Task, signals taskOutcomeSignals) string {
	return fmt.Sprintf(
		"The coder required rework under the %s workflow. Reviewer rejections: %d. Deterministic deliverable violations: %d. Terminal status: %s.",
		signals.patternLabel,
		signals.reviewFailures,
		len(signals.qualityViolations),
		task.Status,
	)
}

func buildReviewerAnalysis(task *taskdomain.Task, signals taskOutcomeSignals, nonLLMDecisions int) string {
	return fmt.Sprintf(
		"The reviewer relied on non-primary decision paths while assessing a %s task. Reviewer protocol violations: %d. Non-LLM decision fallbacks: %d. Terminal status: %s.",
		signals.patternLabel,
		len(signals.protocolViolations),
		nonLLMDecisions,
		task.Status,
	)
}

func buildCoderSuggestions(task *taskdomain.Task, signals taskOutcomeSignals) []string {
	suggestions := make([]string, 0, 5)
	joinedViolations := strings.ToLower(strings.Join(signals.qualityViolations, "\n"))

	if strings.Contains(joinedViolations, "exact prefix") || strings.Contains(joinedViolations, "starting with `") || strings.Contains(joinedViolations, "前缀") {
		suggestions = append(suggestions, "Satisfy exact bullet prefixes in order before improving wording.")
	}
	if strings.Contains(joinedViolations, "meta commentary") || strings.Contains(joinedViolations, "元叙述") {
		suggestions = append(suggestions, "Return only the final deliverable with no preface, explanation, or meta commentary.")
	}
	if strings.Contains(joinedViolations, "bullet-only output") || strings.Contains(joinedViolations, "项目符号输出") || strings.Contains(joinedViolations, "exactly") || strings.Contains(joinedViolations, "项目符号") {
		suggestions = append(suggestions, "Treat explicit bullet-count and bullet-only contracts as hard requirements.")
	}
	if strings.Contains(joinedViolations, "fewer than") || strings.Contains(joinedViolations, "词数") || strings.Contains(joinedViolations, "word") {
		suggestions = append(suggestions, "Count words before finalizing constrained deliverables.")
	}

	if task.WorkflowPattern == taskdomain.PatternReviewCritique || task.WorkflowPattern == taskdomain.PatternIterativeRefinement || task.WorkflowPattern == taskdomain.PatternLoop {
		suggestions = append(suggestions, "In evaluator-optimizer style loops, treat revision feedback as a patch list instead of content to repeat.")
	}
	if len(suggestions) == 0 {
		suggestions = append(suggestions, "Read the explicit output contract literally and satisfy it before adding extra detail.")
	}

	return dedupeStrings(suggestions)
}

func buildReviewerSuggestions(signals taskOutcomeSignals) []string {
	suggestions := make([]string, 0, 3)
	if len(signals.protocolViolations) > 0 {
		suggestions = append(suggestions, "Always include an explicit `Decision: [PASS]` or `Decision: [FAIL]` line.")
	}
	if signals.decisionSourceCounts["repair"] > 0 || signals.decisionSourceCounts["heuristic"] > 0 {
		suggestions = append(suggestions, "Return the review decision correctly on the first pass without relying on repair or heuristic fallback.")
	}
	if signals.decisionSourceCounts["contract_fallback"] > 0 {
		suggestions = append(suggestions, "Judge the deliverable separately from reviewer formatting and do not force contract fallback for compliant output.")
	}
	if len(suggestions) == 0 {
		suggestions = append(suggestions, "Follow the reviewer response contract exactly and emit a clear PASS or FAIL decision.")
	}
	return dedupeStrings(suggestions)
}

func workflowPatternLabel(pattern taskdomain.WorkflowPattern) string {
	switch taskdomain.NormalizeWorkflowPattern(pattern) {
	case taskdomain.PatternSequential:
		return "prompt-chaining / sequential"
	case taskdomain.PatternParallel:
		return "parallelization / parallel"
	case taskdomain.PatternCoordinator:
		return "routing / coordinator"
	case taskdomain.PatternReviewCritique:
		return "evaluator-optimizer / review_critique"
	case taskdomain.PatternIterativeRefinement:
		return "iterative refinement loop"
	case taskdomain.PatternLoop:
		return "bounded loop"
	case taskdomain.PatternHierarchical:
		return "hierarchical orchestration"
	default:
		return string(pattern)
	}
}

func taskOrgID(task *taskdomain.Task, events []*taskdomain.Event) string {
	if task != nil && task.Input != nil {
		if orgID := strings.TrimSpace(stringValue(task.Input["org_id"])); orgID != "" {
			return orgID
		}
	}
	for _, event := range events {
		if event == nil || event.Payload == nil {
			continue
		}
		if orgID := strings.TrimSpace(stringValue(event.Payload["org_id"])); orgID != "" {
			return orgID
		}
	}
	return "default"
}

func roleMatches(name, role string) bool {
	name = strings.TrimSpace(strings.ToLower(name))
	role = strings.TrimSpace(strings.ToLower(role))
	if name == "" || role == "" {
		return false
	}
	return name == role || strings.HasPrefix(name, role+"-")
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return dedupeStrings(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(stringValue(item)); text != "" {
				values = append(values, text)
			}
		}
		return dedupeStrings(values)
	default:
		return nil
	}
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprintf("%v", value)
}

func boolValue(value any) (bool, bool) {
	boolean, ok := value.(bool)
	return boolean, ok
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	default:
		return 0
	}
}

func appendUniqueStrings(base []string, values ...string) []string {
	return dedupeStrings(append(base, values...))
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	deduped := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		deduped = append(deduped, trimmed)
	}
	return deduped
}
