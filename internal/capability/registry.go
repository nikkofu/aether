package capability

import (
	"context"
	"fmt"
	"sync"
)

// CapabilityRegistry 是一个线程安全的能力注册表，支持管理和批量执行。
type CapabilityRegistry struct {
	mu           sync.RWMutex
	capabilities map[string]Capability
}

// NewCapabilityRegistry 创建并返回一个新的 CapabilityRegistry 实例。
func NewCapabilityRegistry() *CapabilityRegistry {
	return &CapabilityRegistry{
		capabilities: make(map[string]Capability),
	}
}

// Register 将一个能力注册到注册表中。如果名称重复，后注册的将覆盖前者。
func (r *CapabilityRegistry) Register(c Capability) {
	if c == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilities[c.Name()] = c
}

// Get 根据名称查找并返回一个已注册的能力。
func (r *CapabilityRegistry) Get(name string) (Capability, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.capabilities[name]
	return c, ok
}

// BatchExecuteResult 存储批量执行中单个任务的结果。
type BatchExecuteResult struct {
	CapName string
	Output  map[string]any
	Error   error
}

// BatchExecute 并发执行多个指定的能力。
// inputs: 键为 Capability 名称，值为传递给该能力的输入参数。
func (r *CapabilityRegistry) BatchExecute(ctx context.Context, inputs map[string]map[string]any) map[string]BatchExecuteResult {
	results := make(map[string]BatchExecuteResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for name, input := range inputs {
		capInstance, ok := r.Get(name)
		if !ok {
			results[name] = BatchExecuteResult{
				CapName: name,
				Error:   fmt.Errorf("capability '%s' not found", name),
			}
			continue
		}

		wg.Add(1)
		go func(n string, c Capability, i map[string]any) {
			defer wg.Done()
			
			output, err := c.Execute(ctx, i)
			
			mu.Lock()
			results[n] = BatchExecuteResult{
				CapName: n,
				Output:  output,
				Error:   err,
			}
			mu.Unlock()
		}(name, capInstance, input)
	}

	wg.Wait()
	return results
}

// ListNames 返回当前所有已注册能力的名称。
func (r *CapabilityRegistry) ListNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	names := make([]string, 0, len(r.capabilities))
	for name := range r.capabilities {
		names = append(names, name)
	}
	return names
}
