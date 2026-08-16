//go:build integration

package stowmark_test

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	testcontainerslog "github.com/testcontainers/testcontainers-go/log"
)

func startSMBContainer(
	t *testing.T,
	ctx context.Context,
) testcontainers.Container {
	t.Helper()

	repositoryPath := t.TempDir()

	smbContainer, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image: "ghcr.io/servercontainers/samba:smbd-only-latest",

				ExposedPorts: []string{
					"445/tcp",
				},

				Env: map[string]string{
					"ACCOUNT_stowmark": "stowmark",
					"UID_stowmark":     strconv.Itoa(os.Getuid()),

					"SAMBA_VOLUME_CONFIG_stowmark": strings.Join(
						[]string{
							"[stowmark]",
							"path = /repositories",
							"valid users = stowmark",
							"guest ok = no",
							"read only = no",
							"browseable = yes",
							"create mask = 0644",
							"directory mask = 0755",
						},
						"; ",
					),
				},

				HostConfigModifier: func(
					hostConfig *container.HostConfig,
				) {
					hostConfig.Mounts = append(
						hostConfig.Mounts,
						mount.Mount{
							Type:     mount.TypeBind,
							Source:   repositoryPath,
							Target:   "/repositories",
							ReadOnly: false,
						},
					)
				},

				WaitingFor: wait.
					ForListeningPort("445/tcp").
					WithStartupTimeout(60 * time.Second),
			},
			Started: true,
			Logger:  testcontainerslog.TestLogger(t),
		},
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(
			t,
			testcontainers.TerminateContainer(smbContainer),
		)
	})

	return smbContainer
}
func smbContainerAddress(t *testing.T, ctx context.Context, container testcontainers.Container) string {
	t.Helper()

	host, err := container.Host(ctx)
	require.NoError(t, err)

	mappedPort, err := container.MappedPort(ctx, "445/tcp")
	require.NoError(t, err)

	hostPort := mappedPort.Port()
	address := net.JoinHostPort(host, hostPort)

	return address
}
