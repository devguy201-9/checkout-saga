// Package logger provides a structured logger based on zap.
//
// Design (ADR-0003):
//   - Interface-based: mockable in tests.
//   - trace_id propagated via context.Context.
//   - JSON encoder for production, console encoder for development.
//   - Every log line in a request scope MUST carry a trace_id field.
package logger

import (
	"context"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is the contract every service uses.
// It does NOT expose *zap.Logger directly — callers must go through the interface.
type Logger interface {
	Debug(msg string, fields ...zap.Field)
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	Fatal(msg string, fields ...zap.Field)
	With(fields ...zap.Field) Logger
	Sync() error
}

// contextKey is an unexported type to avoid collisions with other packages' context keys.
type contextKey struct{ name string }

var (
	traceIDKey = &contextKey{name: "trace_id"}
	loggerKey  = &contextKey{name: "logger"}
)

const (
	// TraceIDField is the standard field name for the trace ID in JSON logs.
	TraceIDField = "trace_id"
	// ServiceField is the standard field name for the service name.
	ServiceField = "service"
)

type zapLogger struct {
	l *zap.Logger
}

// New creates a new logger.
//
// level: "debug" | "info" | "warn" | "error" (defaults to "info" if invalid).
// development=true → console encoder + color + caller path.
// development=false → JSON encoder for production (parsed by Cloud Logging).
func New(level string, development bool) Logger {
	zapLevel := parseLevel(level)

	var cfg zap.Config
	if development {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	} else {
		cfg = zap.NewProductionConfig()
		cfg.EncoderConfig.TimeKey = "timestamp"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		// Cloud Logging convention: "severity" instead of "level".
		cfg.EncoderConfig.LevelKey = "severity"
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	}
	cfg.Level = zap.NewAtomicLevelAt(zapLevel)
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}

	l, err := cfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		// Logger init failed — cannot log via the logger (chicken-and-egg).
		// Write to stderr directly + exit. One of the few places os.Exit outside main is accepted.
		if _, werr := os.Stderr.WriteString("logger init failed: " + err.Error() + "\n"); werr != nil {
			// stderr itself is broken — nothing left to report to, fail loudly.
			panic("logger init failed and stderr is unavailable: " + err.Error() + " / " + werr.Error())
		}
		os.Exit(1)
	}

	return &zapLogger{l: l}
}

// NewWithService attaches a "service" field to every log line.
// Each service creates its own logger with its service name.
func NewWithService(level, service string, development bool) Logger {
	return New(level, development).With(zap.String(ServiceField, service))
}

// WithTraceID injects a trace_id into the context.
// HTTP/gRPC middleware calls this when a request is received.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	if traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDKey, traceID)
}

// TraceIDFromContext reads the trace_id from the context, returning "" if absent.
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(traceIDKey).(string); ok {
		return v
	}
	return ""
}

// WithLogger injects a logger into the context.
// Used when a handler wants to pass a scoped logger to deeper layers.
func WithLogger(ctx context.Context, log Logger) context.Context {
	return context.WithValue(ctx, loggerKey, log)
}

// FromContext returns the logger from the context, enriched with trace_id (if present).
// If the context has no logger, it returns `fallback`.
//
// Usage pattern:
//
//	func (uc *OrderUseCase) Create(ctx context.Context, req Req) error {
//	    log := logger.FromContext(ctx, uc.log)
//	    log.Info("creating order", zap.String("user_id", req.UserID))
//	    ...
//	}
func FromContext(ctx context.Context, fallback Logger) Logger {
	if ctx == nil {
		return fallback
	}
	log := fallback
	if v, ok := ctx.Value(loggerKey).(Logger); ok {
		log = v
	}
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		log = log.With(zap.String(TraceIDField, traceID))
	}
	return log
}

func parseLevel(s string) zapcore.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

func (z *zapLogger) Debug(msg string, fields ...zap.Field) { z.l.Debug(msg, fields...) }
func (z *zapLogger) Info(msg string, fields ...zap.Field)  { z.l.Info(msg, fields...) }
func (z *zapLogger) Warn(msg string, fields ...zap.Field)  { z.l.Warn(msg, fields...) }
func (z *zapLogger) Error(msg string, fields ...zap.Field) { z.l.Error(msg, fields...) }
func (z *zapLogger) Fatal(msg string, fields ...zap.Field) { z.l.Fatal(msg, fields...) }

func (z *zapLogger) With(fields ...zap.Field) Logger {
	return &zapLogger{l: z.l.With(fields...)}
}

func (z *zapLogger) Sync() error {
	// Syncing stdout on a Linux pipe returns an error — safe to ignore.
	return z.l.Sync()
}
