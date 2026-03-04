package capability

import (
	"context"
	"fmt"
)

// Capability 接口定义了 Aether 系统的核心原子能力。
// 任何可以被 Agent 调用的工具、插件或内部功能都应实现此接口。
type Capability interface {
	// Name 返回该能力的唯一标识符。
	Name() string

	// Execute 执行该能力。
	// input: 包含该能力执行所需的所有结构化参数。
	// output: 包含执行结果，成功时返回结果 map，失败时返回错误。
	Execute(ctx context.Context, input map[string]any) (map[string]any, error)
}

// ErrorType 定义了能力的错误类型，用于分类错误。
type ErrorType string

const (
	ErrorTypeInvalidInput ErrorType = "INVALID_INPUT"
	ErrorTypeExecution    ErrorType = "EXECUTION_FAILURE"
	ErrorTypeTimeout      ErrorType = "TIMEOUT"
	ErrorTypeUnknown      ErrorType = "UNKNOWN"
)

// CapabilityError 是一个统一的错误包装器。
type CapabilityError struct {
	Type    ErrorType
	CapName string
	Message string
	Err     error
}

func (e *CapabilityError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] Capability '%s' error: %s (%v)", e.Type, e.CapName, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] Capability '%s' error: %s", e.Type, e.CapName, e.Message)
}

func (e *CapabilityError) Unwrap() error {
	return e.Err
}

// NewError 是一个便捷函数，用于创建 CapabilityError。
func NewError(errType ErrorType, capName string, message string, err error) error {
	return &CapabilityError{
		Type:    errType,
		CapName: capName,
		Message: message,
		Err:     err,
	}
}
