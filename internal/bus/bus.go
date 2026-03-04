package bus

import (
	"context"

	"github.com/nikkofu/aether/internal/agent"
)

// Bus 定义了 Aether 系统的完整消息总线接口
type Bus interface {
	// Publish 发布消息
	Publish(ctx context.Context, msg agent.Message)
	// Subscribe 注册并订阅消息，同时将总线实例注入给 Agent
	Subscribe(a agent.Agent)
	// SubscribeToSubject 订阅特定主题的消息并由回调函数处理
	SubscribeToSubject(ctx context.Context, subject string, handler func(msg agent.Message))
	// Start 启动后台处理循环
	Start(ctx context.Context)
}
