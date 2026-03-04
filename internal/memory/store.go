package memory

import (
	"context"
	"sort"
	"sync"
	"time"
)

// ExecutionRecord 代表一次原子能力执行的持久化记录。
type ExecutionRecord struct {
	// PipelineID 标识该记录所属的流水线执行实例。
	PipelineID string `json:"pipeline_id"`
	// NodeID 标识执行该记录的节点。
	NodeID string `json:"node_id"`
	// Output 包含执行生成的结构化输出数据。
	Output map[string]any `json:"output"`
	// Timestamp 记录执行发生的时间。
	Timestamp time.Time `json:"timestamp"`
}

// Store 定义了 Aether 系统的记忆存储接口，用于持久化任务执行的历史结果。
type Store interface {
	// Save 将一条执行记录保存到存储中。
	Save(ctx context.Context, record ExecutionRecord) error
	// GetByPipeline 返回指定流水线的所有执行记录。
	GetByPipeline(ctx context.Context, pipelineID string) ([]ExecutionRecord, error)
	// ListRecent 返回系统中最近的执行记录，按时间倒序排列。
	ListRecent(ctx context.Context, limit int) ([]ExecutionRecord, error)
	// Close 关闭存储连接并释放资源。
	Close() error
}

// InMemoryStore 是 Store 接口的线程安全内存实现，适用于开发环境或短期会话。
type InMemoryStore struct {
	mu      sync.RWMutex
	records []ExecutionRecord
}

// NewInMemoryStore 创建并返回一个新的 InMemoryStore 实例。
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		records: make([]ExecutionRecord, 0),
	}
}

// Save 实现 Store 接口。
func (s *InMemoryStore) Save(ctx context.Context, record ExecutionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	return nil
}

// GetByPipeline 实现 Store 接口。
func (s *InMemoryStore) GetByPipeline(ctx context.Context, pipelineID string) ([]ExecutionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []ExecutionRecord
	for _, r := range s.records {
		if r.PipelineID == pipelineID {
			result = append(result, r)
		}
	}
	return result, nil
}

// ListRecent 实现 Store 接口。
func (s *InMemoryStore) ListRecent(ctx context.Context, limit int) ([]ExecutionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 复制记录以便排序
	result := make([]ExecutionRecord, len(s.records))
	copy(result, s.records)

	// 按时间戳倒序排列
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// Close 实现 Store 接口。
func (s *InMemoryStore) Close() error {
	return nil
}

// 确保 InMemoryStore 实现了 Store 接口。
var _ Store = (*InMemoryStore)(nil)
