package observability

import (
	"context"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/config"
	otlptracegrpc "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otlptracehttp "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func newTracerProvider(ctx context.Context, cfg config.ObservabilityConfig, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	protocol := observabilityProtocol(cfg)

	var exporter sdktrace.SpanExporter
	var err error

	switch protocol {
	case "http/protobuf":
		exporter, err = otlptracehttp.New(ctx)
	case "grpc":
		exporter, err = otlptracegrpc.New(ctx)
	default:
		return nil, fmt.Errorf("unsupported OTLP protocol %q", protocol)
	}

	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter),
	), nil
}
