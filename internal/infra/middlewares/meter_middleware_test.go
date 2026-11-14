package middlewares_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
	"github.com/bruli-lab/stowmark/internal/infra/observability"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

type testCommand struct{}

func (testCommand) Name() string { return "test-command" }

type testQuery struct{}

func (testQuery) Name() string { return "test-query" }

func collect(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()
	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &collected))
	metrics := map[string]metricdata.Metrics{}
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			metrics[m.Name] = m
		}
	}
	return metrics
}

func statusCounts(t *testing.T, m metricdata.Metrics) map[string]int64 {
	t.Helper()
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok)
	counts := map[string]int64{}
	for _, point := range sum.DataPoints {
		status, ok := point.Attributes.Value(attribute.Key("operation.status"))
		require.True(t, ok)
		counts[status.AsString()] += point.Value
	}
	return counts
}

func TestMeterCommandMiddleware(t *testing.T) {
	t.Parallel()

	t.Run(`Given a command handler wrapped by the meter middleware,
	when it succeeds with events and then fails,
	then it counts the executions by status, records their duration and counts the events`, func(t *testing.T) {
		t.Parallel()

		reader := sdkmetric.NewManualReader()
		middleware, err := middlewares.NewMeterCommandMiddleware(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
		require.NoError(t, err)
		errTest := errors.New("test error")
		calls := 0
		handler := middleware(cqs.CommandHandlerFunc(func(context.Context, cqs.Command) ([]event.Event, error) {
			calls++
			if calls == 1 {
				return []event.Event{snapshot.NewFileRestored("s1", "a"), snapshot.NewFileRestored("s1", "b")}, nil
			}
			return nil, errTest
		}))

		events, err := handler.Handle(context.Background(), testCommand{})
		require.NoError(t, err)
		require.Len(t, events, 2)
		_, err = handler.Handle(context.Background(), testCommand{})
		require.ErrorIs(t, err, errTest)

		metrics := collect(t, reader)
		require.Equal(t, map[string]int64{"success": 1, "error": 1}, statusCounts(t, metrics["stowmark.operation.executions"]))
		histogram, ok := metrics["stowmark.operation.duration"].Data.(metricdata.Histogram[float64])
		require.True(t, ok)
		require.Len(t, histogram.DataPoints, 2)
		eventsSum, ok := metrics["stowmark.command.events"].Data.(metricdata.Sum[int64])
		require.True(t, ok)
		require.Len(t, eventsSum.DataPoints, 1)
		require.Equal(t, int64(2), eventsSum.DataPoints[0].Value)
	})
}

func TestMeterQueryMiddleware(t *testing.T) {
	t.Parallel()

	t.Run(`Given a query handler wrapped by the meter middleware,
	when it succeeds and then fails,
	then it counts the executions by status and returns the results unchanged`, func(t *testing.T) {
		t.Parallel()

		reader := sdkmetric.NewManualReader()
		middleware, err := middlewares.NewMeterQueryMiddleware(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
		require.NoError(t, err)
		errTest := errors.New("test error")
		calls := 0
		handler := middleware(cqs.QueryHandlerFunc(func(context.Context, cqs.Query) (any, error) {
			calls++
			if calls == 1 {
				return "result", nil
			}
			return nil, errTest
		}))

		result, err := handler.Handle(context.Background(), testQuery{})
		require.NoError(t, err)
		require.Equal(t, "result", result)
		_, err = handler.Handle(context.Background(), testQuery{})
		require.ErrorIs(t, err, errTest)

		metrics := collect(t, reader)
		require.Equal(t, map[string]int64{"success": 1, "error": 1}, statusCounts(t, metrics["stowmark.operation.executions"]))
		require.Contains(t, metrics, "stowmark.operation.duration")
	})
}

type recordingDispatcher struct {
	mu     sync.Mutex
	events []event.Event
}

func (r *recordingDispatcher) Dispatch(_ context.Context, ev event.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return nil
}

func testObservability(reader *sdkmetric.ManualReader) *observability.OTLPObservability {
	return observability.NewOTLPObservability(
		tracenoop.NewTracerProvider(),
		sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil,
	)
}

func TestBuildMiddlewares(t *testing.T) {
	t.Parallel()

	t.Run(`Given the command middlewares built from an observability and a dispatcher,
	when a command returns events,
	then the events are dispatched and the execution is measured`, func(t *testing.T) {
		t.Parallel()

		reader := sdkmetric.NewManualReader()
		dispatcher := &recordingDispatcher{}
		middleware, err := middlewares.BuildCommandMiddlewares(testObservability(reader), dispatcher)
		require.NoError(t, err)
		restored := snapshot.NewFileRestored("s1", "a")

		events, err := middleware(cqs.CommandHandlerFunc(func(context.Context, cqs.Command) ([]event.Event, error) {
			return []event.Event{restored}, nil
		})).Handle(context.Background(), testCommand{})

		require.NoError(t, err)
		require.Equal(t, []event.Event{restored}, events)
		require.Equal(t, []event.Event{restored}, dispatcher.events)
		require.Equal(t, map[string]int64{"success": 1}, statusCounts(t, collect(t, reader)["stowmark.operation.executions"]))
	})

	t.Run(`Given the query middlewares built from an observability,
	when a query runs,
	then it returns the result and the execution is measured`, func(t *testing.T) {
		t.Parallel()

		reader := sdkmetric.NewManualReader()
		middleware, err := middlewares.BuildQueryMiddlewares(testObservability(reader))
		require.NoError(t, err)

		result, err := middleware(cqs.QueryHandlerFunc(func(context.Context, cqs.Query) (any, error) {
			return 42, nil
		})).Handle(context.Background(), testQuery{})

		require.NoError(t, err)
		require.Equal(t, 42, result)
		require.Equal(t, map[string]int64{"success": 1}, statusCounts(t, collect(t, reader)["stowmark.operation.executions"]))
	})
}
