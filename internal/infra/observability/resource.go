package observability

import (
	"context"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/config"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
)

const defaultServiceName = "stowmark"

func newResource(ctx context.Context, cfg config.ObservabilityConfig) (*resource.Resource, error) {
	serviceName := defaultServiceName

	if cfg.OTELServiceName != nil && *cfg.OTELServiceName != "" {
		serviceName = *cfg.OTELServiceName
	}

	res, err := resource.New(
		ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			attribute.String("service.name", serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create OpenTelemetry resource: %w", err)
	}

	return res, nil
}
