package observability_test

import (
	"context"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/infra/observability"
	"github.com/stretchr/testify/require"
)

func TestOTLPObservability_Shutdown(t *testing.T) {
	t.Parallel()

	t.Run(`Given an observability whose command context was cancelled by Ctrl-C,
	when Shutdown is called with that context,
	then the exporters receive a live context bounded by the shutdown timeout`, func(t *testing.T) {
		t.Parallel()

		var received context.Context
		obsv := observability.NewOTLPObservability(nil, nil, nil, func(ctx context.Context) error {
			received = ctx
			return ctx.Err()
		})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := obsv.Shutdown(ctx)

		require.NoError(t, err)
		deadline, ok := received.Deadline()
		require.True(t, ok)
		require.WithinDuration(t, time.Now().Add(observability.ShutdownTimeout), deadline, time.Second)
	})

	t.Run(`Given an observability without exporters,
	when Shutdown is called,
	then it does nothing`, func(t *testing.T) {
		t.Parallel()

		require.NoError(t, observability.NewOTLPObservability(nil, nil, nil, nil).Shutdown(context.Background()))
	})
}
