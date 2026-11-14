package app_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func createdEvent() *snapshot.SnapshotCreated {
	manifest := fixtures.ManifestBuilder{ID: new("snapshot-id")}.Build()
	return snapshot.NewSnapshotCreated(&manifest, 3, 42, nil)
}

func restoredEvent() *snapshot.SnapshotRestored {
	result := snapshot.NewResult("snapshot-id")
	for range 4 {
		result.AddSuccess()
	}
	result.AddFailed(*snapshot.NewFailedResult("a", "b"))
	return result.Restored()
}

func listenInSpan(t *testing.T, listener event.Listener, ev event.Event) (sdktrace.ReadOnlySpan, error) {
	t.Helper()

	recorder, provider := newRecorder()
	ctx, span := provider.Tracer("test").Start(context.Background(), "test")
	err := listener.Listen(ctx, ev)
	span.End()

	ended := recorder.Ended()
	require.Len(t, ended, 1)
	return ended[0], err
}

func TestSnapshotTracers_Listen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		listener      event.Listener
		event         event.Event
		expectedAttrs []attribute.KeyValue
	}{
		{
			name:     "a SnapshotCreatedTracer, when Listen is called with a created snapshot event, then it sets the snapshot attributes on the span",
			listener: app.NewSnapshotCreatedTracer(),
			event:    createdEvent(),
			expectedAttrs: []attribute.KeyValue{
				attribute.String("snapshot.id", "snapshot-id"),
				attribute.Int("snapshot.file_count", 3),
				attribute.Int64("snapshot.total_size", 42),
				attribute.Int("snapshot.skipped_files", 0),
			},
		},
		{
			name:     "a SnapshotRestoredTracer, when Listen is called with a restored snapshot event, then it sets the restore attributes on the span",
			listener: app.NewSnapshotRestoredTracer(),
			event:    restoredEvent(),
			expectedAttrs: []attribute.KeyValue{
				attribute.String("snapshot.id", "snapshot-id"),
				attribute.Int("snapshot.total_files_restored", 5),
				attribute.Int("snapshot.total_files_failed", 1),
				attribute.Bool("snapshot.is_success", false),
			},
		},
	}
	for _, tt := range tests {
		t.Run(`Given `+tt.name, func(t *testing.T) {
			t.Parallel()

			span, err := listenInSpan(t, tt.listener, tt.event)

			require.NoError(t, err)
			require.ElementsMatch(t, tt.expectedAttrs, span.Attributes())
		})
	}

	t.Run(`Given a snapshot tracer,
	when Listen is called with an event of another type,
	then it returns an error without setting attributes`, func(t *testing.T) {
		t.Parallel()

		for _, listener := range []event.Listener{app.NewSnapshotCreatedTracer(), app.NewSnapshotRestoredTracer()} {
			span, err := listenInSpan(t, listener, snapshot.NewFileRestored("snapshot-id", "a"))

			require.Error(t, err)
			require.Empty(t, span.Attributes())
		}
	})
}

func TestSpanEventRecorder_Listen(t *testing.T) {
	t.Parallel()

	t.Run(`Given a span event recorder,
	when Listen is called with a domain event,
	then it adds a span event named after the domain event with its aggregate`, func(t *testing.T) {
		t.Parallel()

		ev := snapshot.NewFileRestored("snapshot-id", "a")

		span, err := listenInSpan(t, app.NewSpanEventRecorder(), ev)

		require.NoError(t, err)
		require.Len(t, span.Events(), 1)
		require.Equal(t, snapshot.FileRestoredEventName, span.Events()[0].Name)
		require.Contains(t, span.Events()[0].Attributes, attribute.String("event.aggregate_root_id", "snapshot-id"))
	})
}

func TestEventLogger_Listen(t *testing.T) {
	t.Parallel()

	t.Run(`Given an event logger,
	when Listen is called with a domain event,
	then it logs the event name and aggregate at debug level`, func(t *testing.T) {
		t.Parallel()

		var output bytes.Buffer
		logger := newJSONLogger(&output)

		err := app.NewEventLogger(logger).Listen(context.Background(), snapshot.NewFileRestored("snapshot-id", "a"))

		require.NoError(t, err)
		records := logRecords(t, &output)
		require.Len(t, records, 1)
		require.Equal(t, "DEBUG", records[0]["level"])
		require.Equal(t, snapshot.FileRestoredEventName, records[0]["event"])
		require.Equal(t, "snapshot-id", records[0]["aggregate_root_id"])
	})
}

type dispatcherFunc func(ctx context.Context, ev event.Event) error

func (d dispatcherFunc) Dispatch(ctx context.Context, ev event.Event) error {
	return d(ctx, ev)
}

func TestEventBusCommandMiddleware(t *testing.T) {
	t.Parallel()

	events := []event.Event{createdEvent(), snapshot.NewFileRestored("snapshot-id", "a")}
	errListener := errors.New("listener failed")
	tests := []struct {
		name               string
		handlerErr         error
		dispatchErr        error
		expectedDispatched int
		expectedWarning    bool
	}{
		{name: "that succeeds, then it dispatches every event and returns them", expectedDispatched: 2},
		{name: "that fails, then it dispatches nothing and returns the error", handlerErr: errRepository},
		{
			name:               "whose listeners fail, then it logs a warning and still returns the events",
			dispatchErr:        errListener,
			expectedDispatched: 2,
			expectedWarning:    true,
		},
	}
	for _, tt := range tests {
		t.Run(`Given an event bus command middleware,
		when it handles a command `+tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				output     bytes.Buffer
				dispatched []string
			)
			dispatcher := dispatcherFunc(func(_ context.Context, ev event.Event) error {
				dispatched = append(dispatched, ev.EventName())
				return tt.dispatchErr
			})
			handler := app.NewEventBusCommandMiddleware(dispatcher, newJSONLogger(&output))(commandHandler(events, tt.handlerErr))

			got, err := handler.Handle(context.Background(), namedCommand{})

			require.Len(t, dispatched, tt.expectedDispatched)
			if tt.handlerErr != nil {
				require.ErrorIs(t, err, tt.handlerErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, events, got)
			if tt.expectedWarning {
				records := logRecords(t, &output)
				require.Len(t, records, 2)
				require.Equal(t, "WARN", records[0]["level"])
				return
			}
			require.Empty(t, output.String())
		})
	}
}
