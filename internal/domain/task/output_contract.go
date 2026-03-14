package task

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	exactBulletCountPattern  = regexp.MustCompile(`(?i)\bexactly\s+(\d+)\s+(?:short\s+)?bullet(?:\s+points?)?s?\b`)
	maxWordsPattern          = regexp.MustCompile(`(?i)\b(?:keep(?:\s+the\s+total)?|total|stay|remain)\s+under\s+(\d+)\s+words?\b`)
	noMoreThanWordsPattern   = regexp.MustCompile(`(?i)\b(?:at\s+most|no\s+more\s+than|within)\s+(\d+)\s+words?\b`)
	checklistCountPattern    = regexp.MustCompile(`(?i)\b(?:with|include|containing)\s+(\d+)\s+checks?\b`)
	bulletMustMentionPattern = regexp.MustCompile(`(?i)(['"“‘][^'"”’]+['"”’])\s+bullet\s+must\s+(?:mention|include|state)\s+(['"“‘][^'"”’]+['"”’])`)
	quotedStringPattern      = regexp.MustCompile(`'([^']+)'|"([^"]+)"|“([^”]+)”|‘([^’]+)’`)
	bulletLinePattern        = regexp.MustCompile(`^\s*[-*•]\s+\S`)
	listItemLinePattern      = regexp.MustCompile(`^\s*(?:[-*•]|\d+[.)])\s+\S`)
	wordPattern              = regexp.MustCompile(`[\pL\pN]+`)
)

type BulletPhraseRequirement struct {
	Prefix         string `json:"prefix"`
	RequiredPhrase string `json:"required_phrase"`
}

type OutputContract struct {
	ExactBulletCount         int                       `json:"exact_bullet_count,omitempty"`
	ExactBulletPrefixes      []string                  `json:"exact_bullet_prefixes,omitempty"`
	BulletPhraseRequirements []BulletPhraseRequirement `json:"bullet_phrase_requirements,omitempty"`
	MaxWords                 int                       `json:"max_words,omitempty"`
	ChecklistCount           int                       `json:"checklist_count,omitempty"`
}

func ExtractOutputContract(description string) OutputContract {
	normalized := strings.TrimSpace(description)
	if normalized == "" {
		return OutputContract{}
	}

	return OutputContract{
		ExactBulletCount:         extractFirstPositiveInt(exactBulletCountPattern, normalized),
		ExactBulletPrefixes:      extractExactBulletPrefixes(normalized),
		BulletPhraseRequirements: extractBulletPhraseRequirements(normalized),
		MaxWords:                 maxPositiveInt(extractFirstPositiveInt(maxWordsPattern, normalized), extractFirstPositiveInt(noMoreThanWordsPattern, normalized)),
		ChecklistCount:           extractFirstPositiveInt(checklistCountPattern, normalized),
	}
}

func DescribeOutputContract(contract OutputContract) string {
	return describeOutputContract(contract, true)
}

func DescribeOutputContractEnglish(contract OutputContract) string {
	return describeOutputContract(contract, false)
}

func describeOutputContract(contract OutputContract, chinese bool) string {
	if contract.IsZero() {
		if chinese {
			return "未检测到显式格式约束。"
		}
		return "No explicit output contract was detected."
	}

	parts := make([]string, 0, 5)
	if contract.ExactBulletCount > 0 {
		if chinese {
			parts = append(parts, fmt.Sprintf("必须严格输出 %d 条项目符号", contract.ExactBulletCount))
		} else {
			parts = append(parts, fmt.Sprintf("must output exactly %d bullet points", contract.ExactBulletCount))
		}
	}
	if len(contract.ExactBulletPrefixes) > 0 {
		if chinese {
			parts = append(parts, fmt.Sprintf("项目符号必须依次使用这些精确前缀：%s", strings.Join(contract.ExactBulletPrefixes, "、")))
		} else {
			parts = append(parts, fmt.Sprintf("bullet points must use these exact prefixes in order: %s", strings.Join(contract.ExactBulletPrefixes, ", ")))
		}
	}
	if len(contract.BulletPhraseRequirements) > 0 {
		for _, requirement := range contract.BulletPhraseRequirements {
			if chinese {
				parts = append(parts, fmt.Sprintf("前缀 `%s` 对应的项目符号必须提到 `%s`", requirement.Prefix, requirement.RequiredPhrase))
			} else {
				parts = append(parts, fmt.Sprintf("the bullet starting with `%s` must mention `%s`", requirement.Prefix, requirement.RequiredPhrase))
			}
		}
	}
	if contract.ChecklistCount > 0 {
		if chinese {
			parts = append(parts, fmt.Sprintf("清单必须包含 %d 个检查项", contract.ChecklistCount))
		} else {
			parts = append(parts, fmt.Sprintf("checklist must contain %d items", contract.ChecklistCount))
		}
	}
	if contract.MaxWords > 0 {
		if chinese {
			parts = append(parts, fmt.Sprintf("总词数必须少于 %d", contract.MaxWords))
		} else {
			parts = append(parts, fmt.Sprintf("total word count must stay under %d", contract.MaxWords))
		}
	}

	if chinese {
		return strings.Join(parts, "；")
	}
	return strings.Join(parts, "; ")
}

func (c OutputContract) IsZero() bool {
	return c.ExactBulletCount <= 0 && len(c.ExactBulletPrefixes) == 0 && len(c.BulletPhraseRequirements) == 0 && c.MaxWords <= 0 && c.ChecklistCount <= 0
}

func ValidateOutputAgainstTask(description, output string) []string {
	return ValidateOutputAgainstContract(ExtractOutputContract(description), output)
}

func ValidateOutputAgainstContract(contract OutputContract, output string) []string {
	return validateOutputAgainstContract(contract, output, true)
}

func ValidateOutputAgainstContractEnglish(contract OutputContract, output string) []string {
	return validateOutputAgainstContract(contract, output, false)
}

func validateOutputAgainstContract(contract OutputContract, output string, chinese bool) []string {
	if contract.IsZero() {
		return nil
	}

	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		if chinese {
			return []string{"交付物为空，未满足任务输出要求。"}
		}
		return []string{"The deliverable is empty and does not satisfy the task output requirements."}
	}

	lines := strings.Split(trimmed, "\n")
	bulletLines := make([]string, 0, len(lines))
	listLines := 0
	hasNonListContent := false

	for _, line := range lines {
		current := strings.TrimSpace(line)
		if current == "" {
			continue
		}
		if bulletLinePattern.MatchString(current) {
			bulletLines = append(bulletLines, current)
		}
		if listItemLinePattern.MatchString(current) {
			listLines++
			continue
		}
		hasNonListContent = true
	}

	violations := make([]string, 0, 4)
	if contract.ExactBulletCount > 0 {
		violations = append(violations, buildExactBulletCountViolation(contract.ExactBulletCount, len(bulletLines), chinese)...)
	}
	if len(contract.ExactBulletPrefixes) > 0 {
		if contract.ExactBulletCount == 0 && len(bulletLines) != len(contract.ExactBulletPrefixes) {
			if chinese {
				violations = append(violations, fmt.Sprintf("任务要求按指定前缀输出 %d 条项目符号，实际检测到 %d 条。", len(contract.ExactBulletPrefixes), len(bulletLines)))
			} else {
				violations = append(violations, fmt.Sprintf("The task requires %d bullet points using the specified prefixes, but %d were detected.", len(contract.ExactBulletPrefixes), len(bulletLines)))
			}
		}
		for idx, prefix := range contract.ExactBulletPrefixes {
			if idx >= len(bulletLines) {
				if chinese {
					violations = append(violations, fmt.Sprintf("缺少必须以前缀 `%s` 开头的项目符号。", prefix))
				} else {
					violations = append(violations, fmt.Sprintf("A required bullet point starting with `%s` is missing.", prefix))
				}
				continue
			}
			if !strings.HasPrefix(bulletLines[idx], prefix) {
				if chinese {
					violations = append(violations, fmt.Sprintf("第 %d 条项目符号必须以前缀 `%s` 开头，实际为 `%s`。", idx+1, prefix, bulletLines[idx]))
				} else {
					violations = append(violations, fmt.Sprintf("Bullet %d must start with the exact prefix `%s`, but got `%s`.", idx+1, prefix, bulletLines[idx]))
				}
			}
		}
	}
	if len(contract.BulletPhraseRequirements) > 0 {
		for _, requirement := range contract.BulletPhraseRequirements {
			matchedLine := ""
			for _, bulletLine := range bulletLines {
				if strings.HasPrefix(bulletLine, requirement.Prefix) {
					matchedLine = bulletLine
					break
				}
			}
			if matchedLine == "" {
				continue
			}
			if !strings.Contains(strings.ToLower(matchedLine), strings.ToLower(requirement.RequiredPhrase)) {
				if chinese {
					violations = append(violations, fmt.Sprintf("以前缀 `%s` 开头的项目符号必须提到 `%s`，实际为 `%s`。", requirement.Prefix, requirement.RequiredPhrase, matchedLine))
				} else {
					violations = append(violations, fmt.Sprintf("The bullet starting with `%s` must mention `%s`, but got `%s`.", requirement.Prefix, requirement.RequiredPhrase, matchedLine))
				}
			}
		}
	}
	if (contract.ExactBulletCount > 0 || len(contract.ExactBulletPrefixes) > 0) && hasNonListContent {
		if chinese {
			violations = append(violations, "任务要求项目符号输出，但结果仍包含额外说明性段落或元叙述。")
		} else {
			violations = append(violations, "The task requires bullet-only output, but the result still contains extra prose or meta commentary.")
		}
	}

	if contract.ChecklistCount > 0 && listLines != contract.ChecklistCount {
		if chinese {
			violations = append(violations, fmt.Sprintf("任务要求 %d 个检查项，实际检测到 %d 个列表项。", contract.ChecklistCount, listLines))
		} else {
			violations = append(violations, fmt.Sprintf("The task requires %d checklist items, but %d list items were detected.", contract.ChecklistCount, listLines))
		}
	}

	if contract.MaxWords > 0 {
		wordCount := len(wordPattern.FindAllString(trimmed, -1))
		if wordCount >= contract.MaxWords {
			if chinese {
				violations = append(violations, fmt.Sprintf("任务要求总词数少于 %d，实际约为 %d。", contract.MaxWords, wordCount))
			} else {
				violations = append(violations, fmt.Sprintf("The task requires fewer than %d words, but the output is about %d words.", contract.MaxWords, wordCount))
			}
		}
	}

	return violations
}

func buildExactBulletCountViolation(expected, actual int, chinese bool) []string {
	if expected == actual {
		return nil
	}
	if chinese {
		return []string{fmt.Sprintf("任务要求严格输出 %d 条项目符号，实际检测到 %d 条。", expected, actual)}
	}
	return []string{fmt.Sprintf("The task requires exactly %d bullet points, but %d were detected.", expected, actual)}
}

func extractFirstPositiveInt(pattern *regexp.Regexp, text string) int {
	if pattern == nil || strings.TrimSpace(text) == "" {
		return 0
	}

	matches := pattern.FindStringSubmatch(text)
	if len(matches) < 2 {
		return 0
	}

	value, err := strconv.Atoi(matches[1])
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func extractExactBulletPrefixes(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	normalized := strings.ToLower(text)
	if !strings.Contains(normalized, "exact bullet prefixes") && !strings.Contains(normalized, "精确项目符号前缀") && !strings.Contains(normalized, "精确的项目符号前缀") {
		return nil
	}

	matches := quotedStringPattern.FindAllStringSubmatch(text, -1)
	prefixes := make([]string, 0, len(matches))
	for _, match := range matches {
		candidate := firstNonEmpty(match[1:]...)
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if !strings.HasPrefix(candidate, "-") && !strings.HasPrefix(candidate, "*") && !strings.HasPrefix(candidate, "•") {
			continue
		}
		prefixes = append(prefixes, candidate)
	}

	if len(prefixes) == 0 {
		return nil
	}
	return dedupeStringsPreserveOrder(prefixes)
}

func extractBulletPhraseRequirements(text string) []BulletPhraseRequirement {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	matches := bulletMustMentionPattern.FindAllStringSubmatch(text, -1)
	requirements := make([]BulletPhraseRequirement, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		prefix := unquoteQuotedValue(match[1])
		phrase := unquoteQuotedValue(match[2])
		prefix = strings.TrimSpace(prefix)
		phrase = strings.TrimSpace(phrase)
		if prefix == "" || phrase == "" {
			continue
		}
		requirements = append(requirements, BulletPhraseRequirement{
			Prefix:         prefix,
			RequiredPhrase: phrase,
		})
	}

	if len(requirements) == 0 {
		return nil
	}
	return requirements
}

func unquoteQuotedValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 2 {
		return value
	}
	pairs := [][2]string{
		{"'", "'"},
		{"\"", "\""},
		{"“", "”"},
		{"‘", "’"},
	}
	for _, pair := range pairs {
		if strings.HasPrefix(value, pair[0]) && strings.HasSuffix(value, pair[1]) {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, pair[0]), pair[1]))
		}
	}
	return value
}

func dedupeStringsPreserveOrder(values []string) []string {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func maxPositiveInt(values ...int) int {
	maxValue := 0
	for _, value := range values {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}
