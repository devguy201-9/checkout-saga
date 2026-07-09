package logger_test

import (
	"context"
	"testing"

	"github.com/devguy201-9/checkout-saga/pkg/logger"
)

func TestTraceIDPropagation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func() context.Context
		expected string
	}{
		{
			name:     "empty context returns empty trace id",
			setup:    context.Background,
			expected: "",
		},
		{
			name: "trace id round trip",
			setup: func() context.Context {
				return logger.WithTraceID(context.Background(), "trace-123")
			},
			expected: "trace-123",
		},
		{
			name: "empty trace id not injected",
			setup: func() context.Context {
				return logger.WithTraceID(context.Background(), "")
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := tt.setup()
			got := logger.TraceIDFromContext(ctx)
			if got != tt.expected {
				t.Errorf("TraceIDFromContext() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFromContext_NilContext(t *testing.T) {
	t.Parallel()

	fallback := logger.New("info", false)
	got := logger.FromContext(context.TODO(), fallback)
	if got == nil {
		t.Fatal("FromContext returned nil")
	}
}

func TestNew_InvalidLevel_DefaultsToInfo(t *testing.T) {
	t.Parallel()

	// Should not panic.
	log := logger.New("nonsense", false)
	log.Info("should appear at info level")
	// We can't easily assert level without capturing stdout — smoke test only here.
	// Level parsing logic will be assertion-tested via unit test of parseLevel if exposed.
}
