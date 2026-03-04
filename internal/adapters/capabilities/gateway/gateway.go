package gateway

import (
	"context"
	"time"

	"github.com/nikkofu/aether/internal/pkg/audit"
	"github.com/nikkofu/aether/internal/adapters/capabilities"
	"github.com/nikkofu/aether/internal/adapters/capabilities/registry"
	"github.com/nikkofu/aether/internal/pkg/observability/trace"
	"github.com/nikkofu/aether/internal/pkg/security/rbac"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// DefaultGateway 是能力访问的统筹入口，集成了限流逻辑。
type DefaultGateway struct {
	registry *registry.Registry
	rbac     rbac.RBAC
	audit    audit.Logger
	limiter  *RateLimiter // 注入限流器
	tracer   *trace.TraceEngine
}

func NewDefaultGateway(r *registry.Registry, rb rbac.RBAC, a audit.Logger, l *RateLimiter, t *trace.TraceEngine) *DefaultGateway {
	return &DefaultGateway{
		registry: r,
		rbac:     rb,
		audit:    a,
		limiter:  l,
		tracer:   t,
	}
}

func (g *DefaultGateway) Register(cap capabilities.Capability) {
	g.registry.Register(cap)
}

func (g *DefaultGateway) Execute(ctx context.Context, req capabilities.CapabilityRequest) (capabilities.CapabilityResponse, error) {
	// Tracing: capability call
	if g.tracer != nil {
		var span oteltrace.Span
		ctx, span = g.tracer.StartSpan(ctx, "capability."+req.Name)
		span.SetAttributes(
			attribute.String("org_id", req.OrgID),
			attribute.String("skill_id", req.SkillID),
		)
		defer span.End()
	}

	// 1. 查找能力
	cap, err := g.registry.Get(req.Name)
	if err != nil {
		g.logFailure(ctx, req, "能力未找到", err)
		if g.tracer != nil {
			span := oteltrace.SpanFromContext(ctx)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return capabilities.CapabilityResponse{Success: false, Error: err.Error()}, nil
	}

	// 2. 限流校验 (Rate Limit)
	if g.limiter != nil {
		if !g.limiter.Allow(req.SkillID, req.OrgID) {
			g.logFailure(ctx, req, "触发频率限制", nil)
			if g.tracer != nil {
				span := oteltrace.SpanFromContext(ctx)
				span.SetStatus(codes.Error, "rate limit exceeded")
			}
			return capabilities.CapabilityResponse{Success: false, Error: "rate limit exceeded"}, nil
		}
	}

	// 3. RBAC 权限校验
	perm := rbac.Permission("execute_" + req.Name)
	if !g.rbac.CheckPermission(req.UserID, perm, req.OrgID) {
		g.logFailure(ctx, req, "无权执行此能力", nil)
		if g.tracer != nil {
			span := oteltrace.SpanFromContext(ctx)
			span.SetStatus(codes.Error, "unauthorized")
		}
		return capabilities.CapabilityResponse{Success: false, Error: "unauthorized"}, nil
	}

	// 4. 组织隔离校验
	if req.OrgID == "" {
		g.logFailure(ctx, req, "缺少组织 ID", nil)
		if g.tracer != nil {
			span := oteltrace.SpanFromContext(ctx)
			span.SetStatus(codes.Error, "missing org_id")
		}
		return capabilities.CapabilityResponse{Success: false, Error: "missing org_id"}, nil
	}

	// 5. 审计日志 (Before)
	g.audit.Log(ctx, req.OrgID, audit.EventEconomyCharge, "能力调用启动", map[string]any{"cap": req.Name, "skill": req.SkillID})

	// 6. 执行原子能力
	start := time.Now()
	resp, err := cap.Execute(ctx, req)
	duration := time.Since(start)

	if err != nil && g.tracer != nil {
		span := oteltrace.SpanFromContext(ctx)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	// 7. 审计日志 (After)
	status := "success"
	if err != nil || !resp.Success { status = "fail" }
	g.audit.Log(ctx, req.OrgID, audit.EventEconomyCharge, "能力调用结束", map[string]any{
		"cap": req.Name, "status": status, "duration_ms": duration.Milliseconds(),
	})

	return resp, err
}

func (g *DefaultGateway) logFailure(ctx context.Context, req capabilities.CapabilityRequest, msg string, err error) {
	errStr := ""
	if err != nil { errStr = err.Error() }
	g.audit.Log(ctx, req.OrgID, audit.EventConstRejected, "能力调用拦截", map[string]any{
		"cap": req.Name, "reason": msg, "err": errStr, "skill": req.SkillID,
	})
}
