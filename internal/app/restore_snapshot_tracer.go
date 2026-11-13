package app

import (
	"fmt"
	"log/slog"

	"github.com/bruli-lab/go-core/event"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type RestoreSnapshotTracer struct {
	logger *slog.Logger
}

func (r RestoreSnapshotTracer) Trace(span trace.Span, ev event.Event) {
	restoreEv, ok := ev.(*RestoreSnapshotEvent)
	if !ok {
		r.logger.Error(
			"invalid restore snapshot event",
			slog.String("event", ev.EventName()),
			slog.String("type", fmt.Sprintf("%T", ev)),
		)
		return
	}
	span.SetAttributes(
		attribute.String("snapshot.id", restoreEv.SnapshotID),
		attribute.Int("snapshot.total_files_restored", restoreEv.TotalFiles),
		attribute.Int("snapshot.total_files_failed", len(restoreEv.FailedFiles)),
		attribute.Bool("snapshot.is_success", restoreEv.IsSuccess))
}

func NewRestoreSnapshotTracer(logger *slog.Logger) *RestoreSnapshotTracer {
	return &RestoreSnapshotTracer{logger: logger}
}
