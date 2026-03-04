package security

import (
	"fmt"
	"strings"
)

// SkillValidator 负责对自动生成的代码进行三条铁律的合规性校验。
type SkillValidator struct {
	forbiddenImports []string
	forbiddenCalls   []string
}

func NewSkillValidator() *SkillValidator {
	return &SkillValidator{
		// 严禁技能访问主数据库或执行任意网络请求
		forbiddenImports: []string{
			"database/sql",
			"github.com/nikkofu/aether/internal", // 禁止访问核心模块
			"net/http",
			"os/exec", // 禁止派生子进程
		},
		// 严禁破坏性系统操作
		forbiddenCalls: []string{
			"os.Remove",
			"os.RemoveAll",
			"os.Exit",
			"panic",
		},
	}
}

// ValidateCode 检查源码是否违反安全规则。
func (v *SkillValidator) ValidateCode(code string) error {
	// 1. 检查禁止的导入
	for _, imp := range v.forbiddenImports {
		if strings.Contains(code, fmt.Sprintf("\"%s\"", imp)) {
			return fmt.Errorf("违反安全铁律：技能严禁引用核心或敏感模块 '%s'", imp)
		}
	}

	// 2. 检查禁止的系统调用
	for _, call := range v.forbiddenCalls {
		if strings.Contains(code, call+"(") {
			return fmt.Errorf("违反安全铁律：技能代码包含受限的危险操作 '%s'", call)
		}
	}

	return nil
}
