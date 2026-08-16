//go:build integration

package stowmark_test

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	testcontainerslog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startWebDAVContainer(t *testing.T, ctx context.Context) testcontainers.Container {
	t.Helper()

	config := []byte(`
address: 0.0.0.0
port: 6065
directory: /data
permissions: CRUD
debug: false

users:
  - username: stowmark
    password: stowmark
    permissions: CRUD
`)

	webDAVContainer, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image: "ghcr.io/hacdias/webdav:v5.14.2",
				ExposedPorts: []string{
					"6065/tcp",
				},
				Files: []testcontainers.ContainerFile{
					{
						Reader:            bytes.NewReader(config),
						ContainerFilePath: "/config.yml",
						FileMode:          0o644,
					},
				},
				Cmd: []string{"-c", "/config.yml"},
				HostConfigModifier: func(hostConfig *container.HostConfig) {
					hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
						Type:   mount.TypeTmpfs,
						Target: "/data",
					})
				},
				WaitingFor: wait.
					ForListeningPort("6065/tcp").
					WithStartupTimeout(60 * time.Second),
			},
			Started: true,
			Logger:  testcontainerslog.TestLogger(t),
		},
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, testcontainers.TerminateContainer(webDAVContainer))
	})

	return webDAVContainer
}

func webDAVContainerAddress(t *testing.T, ctx context.Context, webDAVContainer testcontainers.Container) string {
	t.Helper()

	host, err := webDAVContainer.Host(ctx)
	require.NoError(t, err)

	mappedPort, err := webDAVContainer.MappedPort(ctx, "6065/tcp")
	require.NoError(t, err)

	return net.JoinHostPort(
		host,
		mappedPort.Port(),
	)
}
