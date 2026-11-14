package middlewares_test

import (
	"context"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestEventMetricsListener_Listen(t *testing.T) {
	t.Parallel()

	t.Run(`Given an event metrics listener,
	when Listen is called with domain events,
	then it counts the dispatched events by name`, func(t *testing.T) {
		t.Parallel()

		reader := sdkmetric.NewManualReader()
		listener, err := middlewares.NewEventMetricsListener(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
		require.NoError(t, err)

		for range 2 {
			require.NoError(t, listener.Listen(context.Background(), snapshot.NewFileRestored("snapshot-id", "a")))
		}

		var collected metricdata.ResourceMetrics
		require.NoError(t, reader.Collect(context.Background(), &collected))
		require.Len(t, collected.ScopeMetrics, 1)
		require.Len(t, collected.ScopeMetrics[0].Metrics, 1)
		counter := collected.ScopeMetrics[0].Metrics[0]
		require.Equal(t, "stowmark.events.dispatched", counter.Name)
		sum, ok := counter.Data.(metricdata.Sum[int64])
		require.True(t, ok)
		require.Len(t, sum.DataPoints, 1)
		require.Equal(t, int64(2), sum.DataPoints[0].Value)
		name, ok := sum.DataPoints[0].Attributes.Value(attribute.Key("event.name"))
		require.True(t, ok)
		require.Equal(t, snapshot.FileRestoredEventName, name.AsString())
	})
}
