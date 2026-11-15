package observability_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/bruli-lab/stowmark/internal/infra/observability"
	"github.com/stretchr/testify/require"
)

type collector struct {
	mu       sync.Mutex
	requests map[string][]byte
}

func newCollector(t *testing.T) (c *collector, endpoint string) {
	t.Helper()
	c = &collector{requests: map[string][]byte{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.requests[r.URL.Path] = append(c.requests[r.URL.Path], body...)
		c.mu.Unlock()
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return c, server.URL
}

func (c *collector) received(path string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requests[path]
}

func observabilityConfig(endpoint, protocol string) *config.ObservabilityConfig {
	serviceName := "stowmark-test"
	return &config.ObservabilityConfig{
		OTELExporterEndpoint: &endpoint,
		OTELExporterProtocol: &protocol,
		OTELServiceName:      &serviceName,
		LogLevel:             "info",
	}
}

func TestNew_WithHTTPEndpoint(t *testing.T) {
	collected, endpoint := newCollector(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", endpoint)

	t.Run(`Given an observability configuration with an OTLP HTTP endpoint,
	when spans, metrics and logs are recorded and the observability is shut down,
	then they are exported to the collector and the logger honours the log level`, func(t *testing.T) {
		ctx := context.Background()

		obsv, err := observability.New(ctx, observabilityConfig(endpoint, "http/protobuf"))
		require.NoError(t, err)

		_, span := obsv.Tracer("test").Start(ctx, "test-span")
		span.End()
		counter, err := obsv.Meter("test").Int64Counter("test.counter")
		require.NoError(t, err)
		counter.Add(ctx, 1)
		logger := obsv.Logger.With(slog.String("custom-attribute", "custom-value")).WithGroup("group")
		logger.InfoContext(ctx, "exported message", slog.String("inner", "value"))
		require.False(t, obsv.Logger.Enabled(ctx, slog.LevelDebug))
		require.True(t, obsv.Logger.Enabled(ctx, slog.LevelWarn))

		require.NoError(t, obsv.Shutdown(ctx))

		require.True(t, bytes.Contains(collected.received("/v1/traces"), []byte("test-span")))
		require.True(t, bytes.Contains(collected.received("/v1/metrics"), []byte("test.counter")))
		logs := collected.received("/v1/logs")
		require.True(t, bytes.Contains(logs, []byte("exported message")))
		require.True(t, bytes.Contains(logs, []byte("custom-attribute")))
		require.True(t, bytes.Contains(logs, []byte("stowmark-test")))
	})
}

func TestNew_WithGRPCEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4317")

	t.Run(`Given an observability configuration with the grpc protocol,
	when New is called,
	then it creates the providers without connecting to the collector`, func(t *testing.T) {
		obsv, err := observability.New(context.Background(), observabilityConfig("http://127.0.0.1:4317", "grpc"))

		require.NoError(t, err)
		require.NotNil(t, obsv.Tracer("test"))
		require.NotNil(t, obsv.Meter("test"))
		require.NotNil(t, obsv.Logger)
	})
}

func TestNew_WithUnsupportedProtocol(t *testing.T) {
	t.Parallel()

	t.Run(`Given an observability configuration with an unknown protocol,
	when New is called,
	then it returns an unsupported protocol error`, func(t *testing.T) {
		t.Parallel()

		_, err := observability.New(context.Background(), observabilityConfig("http://127.0.0.1:4318", "carrier-pigeon"))

		require.ErrorContains(t, err, `unsupported OTLP protocol "carrier-pigeon"`)
	})
}
