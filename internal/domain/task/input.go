package task

import "strings"

const (
	ParallelBranchesInputKey    = "parallel_branches"
	LegacyParallelTasksInputKey = "parallel_tasks"
	MaxReviewIterationsInputKey = "max_review_iterations"
	defaultMaxReviewIterations  = 3
)

type ParallelBranch struct {
	Name string `json:"name,omitempty"`
	Task string `json:"task"`
}

func NormalizeTaskInput(pattern WorkflowPattern, input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}

	normalized := cloneTaskInputMap(input)
	switch NormalizeWorkflowPattern(pattern) {
	case PatternParallel:
		branches := NormalizeParallelBranches(normalized[ParallelBranchesInputKey])
		if len(branches) == 0 {
			branches = NormalizeParallelBranches(normalized[LegacyParallelTasksInputKey])
		}
		delete(normalized, LegacyParallelTasksInputKey)
		if len(branches) > 0 {
			normalized[ParallelBranchesInputKey] = ParallelBranchesToInput(branches)
		} else {
			delete(normalized, ParallelBranchesInputKey)
		}
	case PatternLoop, PatternReviewCritique, PatternIterativeRefinement:
		if value, ok := normalized[MaxReviewIterationsInputKey]; ok {
			normalized[MaxReviewIterationsInputKey] = normalizePositiveInt(value, defaultMaxReviewIterations)
		}
	}

	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func NormalizeParallelBranches(raw any) []ParallelBranch {
	switch typed := raw.(type) {
	case nil:
		return nil
	case ParallelBranch:
		return compactParallelBranches([]ParallelBranch{typed})
	case []ParallelBranch:
		return compactParallelBranches(typed)
	case []map[string]any:
		result := make([]ParallelBranch, 0, len(typed))
		for _, branch := range typed {
			if normalized, ok := parallelBranchFromMapAny(branch); ok {
				result = append(result, normalized)
			}
		}
		return compactParallelBranches(result)
	case []map[string]string:
		result := make([]ParallelBranch, 0, len(typed))
		for _, branch := range typed {
			if normalized, ok := parallelBranchFromMapString(branch); ok {
				result = append(result, normalized)
			}
		}
		return compactParallelBranches(result)
	case []string:
		result := make([]ParallelBranch, 0, len(typed))
		for _, branch := range typed {
			result = append(result, ParseParallelBranchesText(branch)...)
		}
		return compactParallelBranches(result)
	case string:
		return compactParallelBranches(ParseParallelBranchesText(typed))
	case []any:
		result := make([]ParallelBranch, 0, len(typed))
		for _, item := range typed {
			switch branch := item.(type) {
			case string:
				result = append(result, ParseParallelBranchesText(branch)...)
			case ParallelBranch:
				result = append(result, branch)
			case map[string]any:
				if normalized, ok := parallelBranchFromMapAny(branch); ok {
					result = append(result, normalized)
				}
			case map[string]string:
				if normalized, ok := parallelBranchFromMapString(branch); ok {
					result = append(result, normalized)
				}
			}
		}
		return compactParallelBranches(result)
	default:
		return nil
	}
}

func ParseParallelBranchesText(raw string) []ParallelBranch {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	normalizedSeparators := strings.ReplaceAll(raw, "||", "\n")
	parts := strings.Split(normalizedSeparators, "\n")

	result := make([]ParallelBranch, 0, len(parts))
	for _, part := range parts {
		if branch, ok := parseParallelBranchText(part); ok {
			result = append(result, branch)
		}
	}
	return compactParallelBranches(result)
}

func ParallelBranchesToInput(branches []ParallelBranch) []map[string]any {
	normalized := NormalizeParallelBranches(branches)
	if len(normalized) == 0 {
		return nil
	}

	result := make([]map[string]any, 0, len(normalized))
	for _, branch := range normalized {
		item := map[string]any{
			"task": branch.Task,
		}
		if branch.Name != "" {
			item["name"] = branch.Name
		}
		result = append(result, item)
	}
	return result
}

func parseParallelBranchText(raw string) (ParallelBranch, bool) {
	branch := strings.TrimSpace(raw)
	if branch == "" {
		return ParallelBranch{}, false
	}

	name := ""
	task := branch
	if strings.Contains(branch, "::") {
		parts := strings.SplitN(branch, "::", 2)
		name = strings.TrimSpace(parts[0])
		task = strings.TrimSpace(parts[1])
	}
	if task == "" {
		return ParallelBranch{}, false
	}
	return ParallelBranch{
		Name: strings.TrimSpace(name),
		Task: task,
	}, true
}

func parallelBranchFromMapAny(raw map[string]any) (ParallelBranch, bool) {
	task := firstNonEmptyTaskValue(raw["task"], raw["prompt"], raw["description"])
	if task == "" {
		return ParallelBranch{}, false
	}

	return ParallelBranch{
		Name: strings.TrimSpace(firstNonEmptyTaskValue(raw["name"], raw["title"])),
		Task: task,
	}, true
}

func parallelBranchFromMapString(raw map[string]string) (ParallelBranch, bool) {
	task := strings.TrimSpace(firstNonEmptyStringValue(raw["task"], raw["prompt"], raw["description"]))
	if task == "" {
		return ParallelBranch{}, false
	}

	return ParallelBranch{
		Name: strings.TrimSpace(firstNonEmptyStringValue(raw["name"], raw["title"])),
		Task: task,
	}, true
}

func compactParallelBranches(branches []ParallelBranch) []ParallelBranch {
	if len(branches) == 0 {
		return nil
	}

	result := make([]ParallelBranch, 0, len(branches))
	for _, branch := range branches {
		task := strings.TrimSpace(branch.Task)
		if task == "" {
			continue
		}
		result = append(result, ParallelBranch{
			Name: strings.TrimSpace(branch.Name),
			Task: task,
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizePositiveInt(raw any, fallback int) int {
	switch typed := raw.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int32:
		if typed > 0 {
			return int(typed)
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed >= 1 {
			return int(typed)
		}
	case float32:
		if typed >= 1 {
			return int(typed)
		}
	}
	return fallback
}

func firstNonEmptyTaskValue(values ...any) string {
	for _, value := range values {
		if str, ok := value.(string); ok && strings.TrimSpace(str) != "" {
			return strings.TrimSpace(str)
		}
	}
	return ""
}

func firstNonEmptyStringValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneTaskInputMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}

	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}
