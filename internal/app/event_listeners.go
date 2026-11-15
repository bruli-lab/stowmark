package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type EventLogger struct {
	logger *slog.Logger
}

func (e EventLogger) Listen(ctx context.Context, ev event.Event) error {
	e.logger.DebugContext(ctx, "event dispatched",
		slog.String("event", ev.EventName()),
		slog.String("aggregate_root_id", ev.AggregateRootID()),
	)
	return nil
}

func NewEventLogger(logger *slog.Logger) EventLogger {
	return EventLogger{logger: logger}
}

type SpanEventRecorder struct{}

func (SpanEventRecorder) Listen(ctx context.Context, ev event.Event) error {
	trace.SpanFromContext(ctx).AddEvent(ev.EventName(), trace.WithAttributes(
		attribute.String("event.id", ev.EventID().String()),
		attribute.String("event.aggregate_root_id", ev.AggregateRootID()),
	))
	return nil
}

func NewSpanEventRecorder() SpanEventRecorder {
	return SpanEventRecorder{}
}

type SnapshotCreatedTracer struct{}

func (SnapshotCreatedTracer) Listen(ctx context.Context, ev event.Event) error {
	created, ok := ev.(*snapshot.SnapshotCreated)
	if !ok {
		return unexpectedEventError(snapshot.SnapshotCreatedEventName, ev)
	}
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.String("snapshot.id", created.SnapshotID()),
		attribute.Int("snapshot.file_count", created.FileCount()),
		attribute.Int64("snapshot.total_size", created.TotalSize()),
		attribute.Int("snapshot.skipped_files", len(created.SkippedFiles())),
	)
	return nil
}

func NewSnapshotCreatedTracer() SnapshotCreatedTracer {
	return SnapshotCreatedTracer{}
}

type SnapshotRestoredTracer struct{}

func (SnapshotRestoredTracer) Listen(ctx context.Context, ev event.Event) error {
	restored, ok := ev.(*snapshot.SnapshotRestored)
	if !ok {
		return unexpectedEventError(snapshot.SnapshotRestoredEventName, ev)
	}
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.String("snapshot.id", restored.SnapshotID()),
		attribute.Int("snapshot.total_files_restored", restored.TotalFiles()),
		attribute.Int("snapshot.total_files_failed", len(restored.Failed())),
		attribute.Bool("snapshot.is_success", restored.IsSuccess()),
	)
	return nil
}

func NewSnapshotRestoredTracer() SnapshotRestoredTracer {
	return SnapshotRestoredTracer{}
}

func unexpectedEventError(expected string, ev event.Event) error {
	return fmt.Errorf("listener for %q received %q (%T)", expected, ev.EventName(), ev)
}
