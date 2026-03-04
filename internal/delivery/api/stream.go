package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/pkg/bus"
	"github.com/nikkofu/aether/pkg/logging"
)

// StreamHandler 负责通过 SSE 推送实时系统事件给前端。
type StreamHandler struct {
	bus    bus.Bus
	logger logging.Logger
}

func NewStreamHandler(b bus.Bus, l logging.Logger) *StreamHandler {
	return &StreamHandler{
		bus:    b,
		logger: l,
	}
}

// Handle 建立 SSE 连接并广播 Agent 消息。
func (h *StreamHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// CORS 头处理
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	// 客户端断开时的上下文
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// 创建一个消息接收通道
	msgChan := make(chan agent.Message, 100)

	// 注册一个临时的订阅者到总线，用于监听所有广播事件
	// 这里我们使用 subject "*" 监听所有 Agent 的动态
	h.bus.SubscribeToSubject(ctx, "*", func(msg agent.Message) {
		select {
		case msgChan <- msg:
		default:
			// 缓冲区满时丢弃，避免阻塞总线
		}
	})

	if h.logger != nil {
		h.logger.Info(ctx, "新客户端已连接到实时事件流")
	}

	for {
		select {
		case <-ctx.Done():
			if h.logger != nil {
				h.logger.Info(context.Background(), "客户端断开连接")
			}
			return
		case msg := <-msgChan:
			data, err := json.Marshal(msg)
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", string(data))
				flusher.Flush()
			}
		}
	}
}
