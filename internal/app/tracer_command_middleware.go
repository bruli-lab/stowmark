package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/bruli-lab/stowmark/internal/app"

func NewTracerCommandMiddleware(prov trace.TracerProvider, evTr *EventsTracing) cqs.CommandHandlerMiddleware {
	tracer := prov.Tracer(instrumentationName)
	return func(next cqs.CommandHandler) cqs.CommandHandler {
		return cqs.CommandHandlerFunc(func(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
			ctx, span := tracer.Start(ctx, cmd.Name())
			defer span.End()
			events, err := next.Handle(ctx, cmd)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			for _, ev := range events {
				evTr.Trace(span, ev)
			}
			return events, err
		})
	}
}
