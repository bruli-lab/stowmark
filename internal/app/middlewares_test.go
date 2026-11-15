package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type namedCommand struct{}

func (namedCommand) Name() string { return "test-command" }

type namedQuery struct{}

func (namedQuery) Name() string { return "test-query" }

func commandHandler(events []event.Event, err error) cqs.CommandHandler {
	return cqs.CommandHandlerFunc(func(context.Context, cqs.Command) ([]event.Event, error) {
		return events, err
	})
}

func queryHandler(result any, err error) cqs.QueryHandler {
	return cqs.QueryHandlerFunc(func(context.Context, cqs.Query) (any, error) {
		return result, err
	})
}

func logRecords(t *testing.T, output *bytes.Buffer) []map[string]any {
	t.Helper()

	var records []map[string]any
	decoder := json.NewDecoder(output)
	for decoder.More() {
		record := map[string]any{}
		require.NoError(t, decoder.Decode(&record))
		records = append(records, record)
	}

	return records
}

func newJSONLogger(output *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestLoggerCommandMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		err             error
		expectedLevel   string
		expectedMessage string
		expectedStatus  string
	}{
		{
			name:            "that succeeds, then it logs the start and the completion",
			expectedLevel:   "INFO",
			expectedMessage: "command completed",
			expectedStatus:  "success",
		},
		{
			name:            "that fails, then it logs the start and the failure and returns the error",
			err:             errRepository,
			expectedLevel:   "ERROR",
			expectedMessage: "command failed",
			expectedStatus:  "error",
		},
	}

	for _, tt := range tests {
		t.Run(`Given a logger command middleware,
		when it handles a command `+tt.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			events := []event.Event{snapshot.NewFileRestored("id", "file")}
			handler := app.NewLoggerCommandMiddleware(newJSONLogger(&output))(commandHandler(events, tt.err))

			got, err := handler.Handle(context.Background(), namedCommand{})

			require.ErrorIs(t, err, tt.err)
			require.Equal(t, events, got)
			records := logRecords(t, &output)
			require.Len(t, records, 2)
			require.Equal(t, "command started", records[0]["msg"])
			require.Equal(t, "test-command", records[0]["command"])
			require.Equal(t, tt.expectedLevel, records[1]["level"])
			require.Equal(t, tt.expectedMessage, records[1]["msg"])
			require.Equal(t, tt.expectedStatus, records[1]["status"])
			require.Equal(t, "test-command", records[1]["command"])
			require.InDelta(t, 1, records[1]["events"], 0)
		})
	}
}

func TestLoggerQueryMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		err             error
		expectedLevel   string
		expectedMessage string
		expectedStatus  string
	}{
		{
			name:            "that succeeds, then it logs the start and the completion",
			expectedLevel:   "INFO",
			expectedMessage: "query completed",
			expectedStatus:  "success",
		},
		{
			name:            "that fails, then it logs the start and the failure and returns the error",
			err:             errRepository,
			expectedLevel:   "ERROR",
			expectedMessage: "query failed",
			expectedStatus:  "error",
		},
	}

	for _, tt := range tests {
		t.Run(`Given a logger query middleware,
		when it handles a query `+tt.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			handler := app.NewLoggerQueryMiddleware(newJSONLogger(&output))(queryHandler("result", tt.err))

			got, err := handler.Handle(context.Background(), namedQuery{})

			require.ErrorIs(t, err, tt.err)
			require.Equal(t, "result", got)
			records := logRecords(t, &output)
			require.Len(t, records, 2)
			require.Equal(t, "query started", records[0]["msg"])
			require.Equal(t, "test-query", records[0]["query"])
			require.Equal(t, tt.expectedLevel, records[1]["level"])
			require.Equal(t, tt.expectedMessage, records[1]["msg"])
			require.Equal(t, tt.expectedStatus, records[1]["status"])
			require.Equal(t, "test-query", records[1]["query"])
		})
	}
}

func newRecorder() (*tracetest.SpanRecorder, *sdktrace.TracerProvider) {
	recorder := tracetest.NewSpanRecorder()
	return recorder, sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
}

func TestTracerCommandMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		err            error
		expectedStatus codes.Code
	}{
		{
			name:           "that succeeds, then it records a span named after the command",
			expectedStatus: codes.Unset,
		},
		{
			name:           "that fails, then it records an error span",
			err:            errRepository,
			expectedStatus: codes.Error,
		},
	}

	for _, tt := range tests {
		t.Run(`Given a tracer command middleware,
		when it handles a command `+tt.name, func(t *testing.T) {
			t.Parallel()

			recorder, provider := newRecorder()
			handler := app.NewTracerCommandMiddleware(provider)(commandHandler(nil, tt.err))

			_, err := handler.Handle(context.Background(), namedCommand{})

			require.ErrorIs(t, err, tt.err)
			spans := recorder.Ended()
			require.Len(t, spans, 1)
			require.Equal(t, "test-command", spans[0].Name())
			require.Equal(t, tt.expectedStatus, spans[0].Status().Code)
		})
	}
}

func TestTracerQueryMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		err            error
		expectedStatus codes.Code
		expectedEvents int
	}{
		{
			name:           "that succeeds, then it records a span without errors",
			expectedStatus: codes.Unset,
		},
		{
			name:           "that fails, then it records an error span with the error event",
			err:            errRepository,
			expectedStatus: codes.Error,
			expectedEvents: 1,
		},
	}

	for _, tt := range tests {
		t.Run(`Given a tracer query middleware,
		when it handles a query `+tt.name, func(t *testing.T) {
			t.Parallel()

			recorder, provider := newRecorder()
			handler := app.NewTracerQueryMiddleware(provider)(queryHandler("result", tt.err))

			got, err := handler.Handle(context.Background(), namedQuery{})

			require.ErrorIs(t, err, tt.err)
			require.Equal(t, "result", got)
			spans := recorder.Ended()
			require.Len(t, spans, 1)
			require.Equal(t, "test-query", spans[0].Name())
			require.Equal(t, tt.expectedStatus, spans[0].Status().Code)
			require.Len(t, spans[0].Events(), tt.expectedEvents)
		})
	}
}
