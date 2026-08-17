//go:build integration

package stowmark_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	testcontainerslog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/api/option"
)

func startGCSContainer(t *testing.T, ctx context.Context) testcontainers.Container {
	t.Helper()

	gcsContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "fsouza/fake-gcs-server:1.55.1",
			ExposedPorts: []string{"4443/tcp"},
			Cmd:          []string{"-scheme", "http"},
			WaitingFor: wait.ForHTTP("/_internal/healthcheck").
				WithPort("4443/tcp").
				WithStartupTimeout(60 * time.Second),
		},
		Started: true,
		Logger:  testcontainerslog.TestLogger(t),
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, testcontainers.TerminateContainer(gcsContainer))
	})

	return gcsContainer
}

func gcsContainerEndpoint(t *testing.T, ctx context.Context, gcsContainer testcontainers.Container) string {
	t.Helper()

	host, err := gcsContainer.Host(ctx)
	require.NoError(t, err)

	mappedPort, err := gcsContainer.MappedPort(ctx, "4443/tcp")
	require.NoError(t, err)

	address := net.JoinHostPort(host, mappedPort.Port())

	return fmt.Sprintf("http://%s/storage/v1/", address)
}

func createGCSBucket(t *testing.T, ctx context.Context, endpoint, bucket string) {
	t.Helper()

	client, err := storage.NewClient(ctx,
		option.WithEndpoint(endpoint),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	err = client.Bucket(bucket).Create(ctx, "stowmark-test", nil)
	require.NoError(t, err)
}
