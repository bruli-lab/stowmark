package app

import (
	"log/slog"

	"github.com/bruli-lab/go-core/event"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type CreateSnapshotTracer struct {
	logger *slog.Logger
}

func (c CreateSnapshotTracer) Trace(span trace.Span, ev event.Event) {
	createEv, _ := ev.(*CreateSnapshotEvent)
	slog.Error(
		"invalid create snapshot event",
		slog.String("event", ev.EventName()),
		slog.String("snapshot_id", createEv.SnapshotID),
	)
	span.SetAttributes(
		attribute.String("snapshot.id", createEv.SnapshotID),
		attribute.Int("snapshot.file_count", createEv.FileCount),
		attribute.Int64("snapshot.total_size", createEv.TotalSize),
	)
}

func NewCreateSnapshotTracer(logger *slog.Logger) *CreateSnapshotTracer {
	return &CreateSnapshotTracer{logger: logger}
}
