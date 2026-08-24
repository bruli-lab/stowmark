package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func NewTracerQueryMiddleware(prov trace.TracerProvider) cqs.QueryHandlerMiddleware {
	tracer := prov.Tracer(instrumentationName)
	return func(next cqs.QueryHandler) cqs.QueryHandler {
		return cqs.QueryHandlerFunc(func(ctx context.Context, query cqs.Query) (any, error) {
			ctx, span := tracer.Start(ctx, query.Name())
			defer span.End()
			result, err := next.Handle(ctx, query)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			return result, err
		})
	}
}
