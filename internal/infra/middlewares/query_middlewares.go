package middlewares

import (
	"fmt"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/app"
	observabilityinfra "github.com/bruli-lab/stowmark/internal/infra/observability"
)

func BuildQueryMiddlewares(obsv *observabilityinfra.OTLPObservability) (cqs.QueryHandlerMiddleware, error) {
	loggerMdw := app.NewLoggerQueryMiddleware(obsv.Logger)
	tracerMdw := app.NewTracerQueryMiddleware(obsv.TracerProvider)
	meterMdw, err := app.NewMeterQueryMiddleware(obsv.MeterProvider)
	if err != nil {
		return nil, fmt.Errorf("create meter middleware: %w", err)
	}
	multiMdw := cqs.QueryHandlerMultiMiddleware(loggerMdw, tracerMdw, meterMdw)
	return multiMdw, nil
}
