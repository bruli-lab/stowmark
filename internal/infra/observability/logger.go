package observability

import (
	"context"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/config"
	otlploggrpc "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otlploghttp "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

func newLoggerProvider(ctx context.Context, cfg config.ObservabilityConfig, res *resource.Resource) (*sdklog.LoggerProvider, error) {
	protocol := observabilityProtocol(cfg)

	var exporter sdklog.Exporter
	var err error

	switch protocol {
	case "http/protobuf":
		exporter, err = otlploghttp.New(ctx)
	case "grpc":
		exporter, err = otlploggrpc.New(ctx)
	default:
		return nil, fmt.Errorf("unsupported OTLP protocol %q", protocol)
	}

	if err != nil {
		return nil, fmt.Errorf("create OTLP log exporter: %w", err)
	}

	processor := sdklog.NewBatchProcessor(exporter)

	return sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(processor),
	), nil
}
