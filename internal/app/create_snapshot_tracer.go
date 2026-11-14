package app

import (
	"fmt"
	"log/slog"

	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type CreateSnapshotTracer struct {
	logger *slog.Logger
}

func (c CreateSnapshotTracer) Trace(span trace.Span, ev event.Event) {
	createEv, ok := ev.(*snapshot.SnapshotCreated)
	if !ok {
		c.logger.Error(
			"invalid create snapshot event",
			slog.String("event", ev.EventName()),
			slog.String("type", fmt.Sprintf("%T", ev)),
		)
		return
	}
	span.SetAttributes(
		attribute.String("snapshot.id", createEv.SnapshotID()),
		attribute.Int("snapshot.file_count", createEv.FileCount()),
		attribute.Int64("snapshot.total_size", createEv.TotalSize()),
		attribute.Int("snapshot.skipped_files", len(createEv.SkippedFiles())),
	)
}

func NewCreateSnapshotTracer(logger *slog.Logger) *CreateSnapshotTracer {
	return &CreateSnapshotTracer{logger: logger}
}
