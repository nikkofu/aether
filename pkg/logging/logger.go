package logging

import (
	"context"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.opentelemetry.io/otel/trace"
)

// Field 是 zap.Field 的别名。
type Field = zap.Field

// Logger 定义了结构化日志接口。
type Logger interface {
	Debug(ctx context.Context, msg string, fields ...Field)
	Info(ctx context.Context, msg string, fields ...Field)
	Warn(ctx context.Context, msg string, fields ...Field)
	Error(ctx context.Context, msg string, fields ...Field)
	Sync() error
}

// ZapLogger 是 Logger 接口的 zap 实现。
type ZapLogger struct {
	l *zap.Logger
}

// Config 定义了日志系统的初始化配置。
type Config struct {
	Level  string
	Format string // "json" 或 "console"
}

// NewLogger 创建一个新的 ZapLogger 实例。
func NewLogger(cfg Config) (*ZapLogger, error) {
	var encoderCfg zapcore.EncoderConfig
	
	if cfg.Format == "json" {
		encoderCfg = zap.NewProductionEncoderConfig()
	} else {
		encoderCfg = zap.NewDevelopmentEncoderConfig()
		encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	// 1. 优化时间戳：使用人类可读的 ISO8601 格式
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	var zapCfg zap.Config
	if cfg.Format == "json" {
		zapCfg = zap.NewProductionConfig()
		zapCfg.EncoderConfig = encoderCfg
	} else {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.EncoderConfig = encoderCfg
	}

	// 2. 设置日志级别
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level = zap.InfoLevel
	}
	zapCfg.Level = zap.NewAtomicLevelAt(level)

	// 3. 核心修复：AddCallerSkip(1) 确保 caller 指向真正的调用出处而非封装层
	l, err := zapCfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		return nil, err
	}

	return &ZapLogger{l: l}, nil
}

// extractTraceInfo 从 context 中提取 OpenTelemetry 的追踪信息。
func (zl *ZapLogger) extractTraceInfo(ctx context.Context) []Field {
	if ctx == nil {
		return nil
	}
	
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return nil
	}

	fields := make([]Field, 0, 2)
	fields = append(fields, zap.String("trace_id", span.SpanContext().TraceID().String()))
	fields = append(fields, zap.String("span_id", span.SpanContext().SpanID().String()))
	
	return fields
}

func (zl *ZapLogger) Debug(ctx context.Context, msg string, fields ...Field) {
	zl.l.Debug(msg, append(fields, zl.extractTraceInfo(ctx)...)...)
}

func (zl *ZapLogger) Info(ctx context.Context, msg string, fields ...Field) {
	zl.l.Info(msg, append(fields, zl.extractTraceInfo(ctx)...)...)
}

func (zl *ZapLogger) Warn(ctx context.Context, msg string, fields ...Field) {
	zl.l.Warn(msg, append(fields, zl.extractTraceInfo(ctx)...)...)
}

func (zl *ZapLogger) Error(ctx context.Context, msg string, fields ...Field) {
	zl.l.Error(msg, append(fields, zl.extractTraceInfo(ctx)...)...)
}

func (zl *ZapLogger) Sync() error {
	return zl.l.Sync()
}

// 辅助函数
func String(key, val string) Field                 { return zap.String(key, val) }
func Int(key string, val int) Field                    { return zap.Int(key, val) }
func Float64(key string, val float64) Field            { return zap.Float64(key, val) }
func Duration(key string, val time.Duration) Field     { return zap.Duration(key, val) }
func Any(key string, val any) Field                    { return zap.Any(key, val) }
func Err(err error) Field                              { return zap.Error(err) }
