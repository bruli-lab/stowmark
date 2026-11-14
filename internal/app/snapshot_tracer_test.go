package app_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func traceEvent(t *testing.T, tracer app.EventTracer, ev event.Event) []attribute.KeyValue {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	_, span := provider.Tracer("test").Start(context.Background(), "test")
	tracer.Trace(span, ev)
	span.End()

	ended := recorder.Ended()
	require.Len(t, ended, 1)

	return ended[0].Attributes()
}

func createdEvent() *snapshot.SnapshotCreated {
	manifest := fixtures.ManifestBuilder{ID: new("snapshot-id")}.Build()
	return snapshot.NewSnapshotCreated(&manifest, 3, 42)
}

func restoredEvent() *snapshot.SnapshotRestored {
	result := snapshot.NewResult("snapshot-id")
	for range 4 {
		result.AddSuccess()
	}
	result.AddFailed(*snapshot.NewFailedResult("a", "b"))
	return result.Restored()
}

func TestSnapshotTracers_Trace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		tracer        func(logger *slog.Logger) app.EventTracer
		event         event.Event
		expectedAttrs []attribute.KeyValue
		expectedLog   string
	}{
		{
			name: "a CreateSnapshotTracer, when Trace is called with a create snapshot event, then it sets the snapshot attributes",
			tracer: func(logger *slog.Logger) app.EventTracer {
				return app.NewCreateSnapshotTracer(logger)
			},
			event: createdEvent(),
			expectedAttrs: []attribute.KeyValue{
				attribute.String("snapshot.id", "snapshot-id"),
				attribute.Int("snapshot.file_count", 3),
				attribute.Int64("snapshot.total_size", 42),
			},
		},
		{
			name: "a CreateSnapshotTracer, when Trace is called with another event, then it logs an error without setting attributes",
			tracer: func(logger *slog.Logger) app.EventTracer {
				return app.NewCreateSnapshotTracer(logger)
			},
			event:       snapshot.NewFileRestored("snapshot-id", "a"),
			expectedLog: "invalid create snapshot event",
		},
		{
			name: "a RestoreSnapshotTracer, when Trace is called with a restore snapshot event, then it sets the snapshot attributes",
			tracer: func(logger *slog.Logger) app.EventTracer {
				return app.NewRestoreSnapshotTracer(logger)
			},
			event: restoredEvent(),
			expectedAttrs: []attribute.KeyValue{
				attribute.String("snapshot.id", "snapshot-id"),
				attribute.Int("snapshot.total_files_restored", 5),
				attribute.Int("snapshot.total_files_failed", 1),
				attribute.Bool("snapshot.is_success", false),
			},
		},
		{
			name: "a RestoreSnapshotTracer, when Trace is called with another event, then it logs an error without setting attributes",
			tracer: func(logger *slog.Logger) app.EventTracer {
				return app.NewRestoreSnapshotTracer(logger)
			},
			event:       createdEvent(),
			expectedLog: "invalid restore snapshot event",
		},
	}

	for _, tt := range tests {
		t.Run(`Given `+tt.name, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, nil))

			attrs := traceEvent(t, tt.tracer(logger), tt.event)

			if tt.expectedLog != "" {
				require.Empty(t, attrs)
				require.Contains(t, logs.String(), tt.expectedLog)
				return
			}
			require.ElementsMatch(t, tt.expectedAttrs, attrs)
			require.Empty(t, logs.String())
		})
	}
}
