package cli

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/config"
	observabilityinfra "github.com/bruli-lab/stowmark/internal/infra/observability"
)

func builtObservability(ctx context.Context) (*observabilityinfra.OTLPObservability, error) {
	conf := config.NewObservabilityConfig()
	obsv, err := observabilityinfra.New(ctx, conf)
	if err != nil {
		return nil, err
	}
	return obsv, nil
}
