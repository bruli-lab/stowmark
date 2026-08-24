package observability

import (
	"context"
	"fmt"
	"time"

	"github.com/bruli-lab/stowmark/internal/config"
	otlpmetricgrpc "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	otlpmetrichttp "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

const (
	metricExportInterval = 5 * time.Second
	metricExportTimeout  = 3 * time.Second
)

func newMeterProvider(
	ctx context.Context,
	cfg config.ObservabilityConfig,
	res *resource.Resource,
) (*sdkmetric.MeterProvider, error) {
	protocol := observabilityProtocol(cfg)

	var exporter sdkmetric.Exporter
	var err error

	switch protocol {
	case "http/protobuf":
		exporter, err = otlpmetrichttp.New(ctx)
	case "grpc":
		exporter, err = otlpmetricgrpc.New(ctx)
	default:
		return nil, fmt.Errorf(
			"unsupported OTLP protocol %q",
			protocol,
		)
	}

	if err != nil {
		return nil, fmt.Errorf(
			"create OTLP metric exporter: %w",
			err,
		)
	}

	reader := sdkmetric.NewPeriodicReader(
		exporter,
		sdkmetric.WithInterval(metricExportInterval),
		sdkmetric.WithTimeout(metricExportTimeout),
	)

	return sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	), nil
}
