package capability

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

var varRegex = regexp.MustCompile(`{{([a-zA-Z0-9_]+\.[a-zA-Z0-9_]+)}}`)

// PromptRenderer 定义了 Prompt 渲染器的接口。
type PromptRenderer interface {
	Render(name string, templateStr string, data map[string]any) (string, error)
}

// DefaultRenderer 是 PromptRenderer 的默认实现。
type DefaultRenderer struct {
	// Funcs 包含可用于模板的自定义函数。
	Funcs template.FuncMap
}

// NewDefaultRenderer 创建并返回一个带有标准辅助函数的渲染器。
func NewDefaultRenderer() *DefaultRenderer {
	return &DefaultRenderer{
		Funcs: template.FuncMap{
			"trim":  strings.TrimSpace,
			"upper": strings.ToUpper,
			"lower": strings.ToLower,
			"join":  strings.Join,
			"get": func(m any, key string) any {
				if mm, ok := m.(map[string]any); ok {
					return mm[key]
				}
				if mm, ok := m.(map[string]map[string]any); ok {
					return mm[key]
				}
				return nil
			},
			// 安全性辅助：简单的转义或清理，防止输入破坏 Prompt 结构
			"quote": func(s string) string {
				return fmt.Sprintf("%q", s)
			},
			"safe": func(s string) string {
				// 移除可能引起注入的字符（根据具体 LLM 需求可扩展）
				r := strings.NewReplacer("{", "(", "}", ")", "[", "(", "]", ")")
				return r.Replace(s)
			},
		},
	}
}

// Render 动态渲染模板并生成 Prompt 字符串。
func (r *DefaultRenderer) Render(name string, templateStr string, data map[string]any) (string, error) {
	if templateStr == "" {
		return "", fmt.Errorf("模板字符串不能为空")
	}

	// 预处理：将 {{plan.output}} 转换为 {{.plan.output}}，以便符合 Go 模板语法
	processedTmpl := varRegex.ReplaceAllString(templateStr, "{{.$1}}")

	tmpl, err := template.New(name).Funcs(r.Funcs).Parse(processedTmpl)
	if err != nil {
		return "", fmt.Errorf("模板解析失败: %w", err)
	}

	var buf bytes.Buffer
	// 执行模板渲染
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return "", fmt.Errorf("模板渲染失败: %w", err)
	}

	return buf.String(), nil
}

// TemplateManager 管理一组预定义的模板，支持默认和自定义模板。
type TemplateManager struct {
	renderer  PromptRenderer
	templates map[string]string
}

// NewTemplateManager 创建一个新的模板管理器。
func NewTemplateManager(renderer PromptRenderer) *TemplateManager {
	if renderer == nil {
		renderer = NewDefaultRenderer()
	}
	return &TemplateManager{
		renderer:  renderer,
		templates: make(map[string]string),
	}
}

// Register 注册或更新一个预定义的模板。
func (tm *TemplateManager) Register(name string, templateStr string) {
	tm.templates[name] = templateStr
}

// Execute 根据预定义的模板名称渲染 Prompt。
func (tm *TemplateManager) Execute(name string, data map[string]any) (string, error) {
	tmplStr, ok := tm.templates[name]
	if !ok {
		return "", fmt.Errorf("未找到名为 '%s' 的模板", name)
	}
	return tm.renderer.Render(name, tmplStr, data)
}

// ExecuteWithFallback 使用自定义模板渲染，如果自定义模板为空则使用预定义的默认模板。
func (tm *TemplateManager) ExecuteWithFallback(name string, customTmpl string, data map[string]any) (string, error) {
	tmplStr := customTmpl
	if tmplStr == "" {
		var ok bool
		tmplStr, ok = tm.templates[name]
		if !ok {
			return "", fmt.Errorf("未提供自定义模板且未找到默认模板 '%s'", name)
		}
	}
	return tm.renderer.Render(name, tmplStr, data)
}
