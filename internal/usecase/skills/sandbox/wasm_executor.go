package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/nikkofu/aether/internal/infrastructure/capabilities"
	"github.com/nikkofu/aether/pkg/observability/trace"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// WASMExecutor 实现了基于 wazero 的高性能安全沙箱，并支持能力网关对接。
type WASMExecutor struct {
	runtime wazero.Runtime
	gateway capabilities.Gateway
	tracer  *trace.TraceEngine
}

func NewWASMExecutor(ctx context.Context, g capabilities.Gateway, t *trace.TraceEngine) (*WASMExecutor, error) {
	rt := wazero.NewRuntime(ctx)

	// 1. 初始化受限的 WASI
	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	return &WASMExecutor{runtime: rt, gateway: g, tracer: t}, nil
}

func (e *WASMExecutor) Execute(ctx context.Context, orgID, userID, skillID, wasmPath string, input []byte) (output []byte, err error) {
	// Tracing: wasm execution using OpenTelemetry
	if e.tracer != nil {
		var span oteltrace.Span
		ctx, span = e.tracer.StartSpan(ctx, "wasm.execute")
		span.SetAttributes(
			attribute.String("skill_id", skillID),
			attribute.String("org_id", orgID),
			attribute.String("user_id", userID),
			attribute.String("wasm_path", wasmPath),
		)
		defer span.End()

		// Record error if it happens
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
		}()
	}

	childCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var callCount int
	const maxCalls = 5

	// 2. 在每次执行前动态创建 Host 模块以注入上下文
	_, err = e.runtime.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, offset, byteCount uint32) {
			buf, _ := m.Memory().Read(offset, byteCount)
			fmt.Printf("[WASM LOG]: %s\n", string(buf))
		}).Export("console_log").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, nameOffset, nameLen, paramsOffset, paramsLen, retOffset, retLen uint32) {
			// 限制调用次数
			if callCount >= maxCalls { return }
			callCount++

			// 读取能力名称和参数
			nameBuf, _ := m.Memory().Read(nameOffset, nameLen)
			paramsBuf, _ := m.Memory().Read(paramsOffset, paramsLen)

			var params map[string]any
			json.Unmarshal(paramsBuf, &params)

			// 调用网关
			req := capabilities.CapabilityRequest{
				OrgID: orgID, UserID: userID, SkillID: skillID,
				Name: string(nameBuf), Params: params, RequestedAt: time.Now(),
			}
			resp, _ := e.gateway.Execute(ctx, req)

			// 写回结果
			respBuf, _ := json.Marshal(resp)
			m.Memory().Write(retOffset, respBuf)
		}).Export("request_capability").
		Instantiate(childCtx)

	if err != nil { return nil, err }

	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil { return nil, err }

	stdout := &bytes.Buffer{}
	config := wazero.NewModuleConfig().
		WithStdin(bytes.NewReader(input)).
		WithStdout(stdout).
		WithStderr(io.Discard)

	mod, err := e.runtime.InstantiateWithConfig(childCtx, wasmBytes, config)
	if err != nil { return nil, fmt.Errorf("instantiate fail: %w", err) }
	defer mod.Close(childCtx)

	return stdout.Bytes(), nil
}
