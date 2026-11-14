package observability_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/bruli-lab/stowmark/internal/infra/observability"
	"github.com/stretchr/testify/require"
)

func TestNew_WithoutEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		level    string
		enabled  slog.Level
		disabled slog.Level
	}{
		{name: "and the error log level, then the logger only writes errors", level: "error", enabled: slog.LevelError, disabled: slog.LevelWarn},
		{name: "and the debug log level, then the logger writes debug messages", level: "debug", enabled: slog.LevelDebug, disabled: slog.LevelDebug - 1},
		{name: "and an unknown log level, then the logger writes info messages", level: "loud", enabled: slog.LevelInfo, disabled: slog.LevelDebug},
	}
	for _, tt := range tests {
		t.Run(`Given an observability configuration without an OTLP endpoint,
		when New is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			obsv, err := observability.New(context.Background(), &config.ObservabilityConfig{LogLevel: tt.level})

			require.NoError(t, err)
			require.True(t, obsv.Logger.Enabled(context.Background(), tt.enabled))
			require.False(t, obsv.Logger.Enabled(context.Background(), tt.disabled))
			require.NoError(t, obsv.Shutdown(context.Background()))
		})
	}
}
