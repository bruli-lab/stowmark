//go:build integration

package stowmark_test

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	testcontainerslog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func startSSHContainer(t *testing.T, ctx context.Context, publicKey []byte) testcontainers.Container {
	t.Helper()

	repositoriesPath := t.TempDir()

	containerSSH, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image: "lscr.io/linuxserver/openssh-server:10.3_p1-r2-ls243",

				ExposedPorts: []string{
					"2222/tcp",
				},

				Env: map[string]string{
					"PUID":            strconv.Itoa(os.Getuid()),
					"PGID":            strconv.Itoa(os.Getgid()),
					"TZ":              "Europe/Madrid",
					"USER_NAME":       "stowmark",
					"PUBLIC_KEY_FILE": "/keys/stowmark.pub",
					"PASSWORD_ACCESS": "false",
					"SUDO_ACCESS":     "false",
					"LOG_STDOUT":      "true",
				},

				Files: []testcontainers.ContainerFile{
					{
						Reader:            bytes.NewReader(publicKey),
						ContainerFilePath: "/keys/stowmark.pub",
						FileMode:          0o644,
					},
				},

				HostConfigModifier: func(
					hostConfig *container.HostConfig,
				) {
					hostConfig.Mounts = append(
						hostConfig.Mounts,
						mount.Mount{
							Type:     mount.TypeBind,
							Source:   repositoriesPath,
							Target:   "/repositories",
							ReadOnly: false,
						},
					)
				},

				WaitingFor: wait.
					ForListeningPort("2222/tcp").
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
			testcontainers.TerminateContainer(containerSSH),
		)
	})

	return containerSSH
}

func sshContainerAddress(t *testing.T, ctx context.Context, container testcontainers.Container) string {
	t.Helper()

	host, err := container.Host(ctx)
	require.NoError(t, err)

	mappedPort, err := container.MappedPort(ctx, "2222/tcp")
	require.NoError(t, err)

	hostPort := mappedPort.Port()
	address := net.JoinHostPort(host, hostPort)

	return address
}

func createKnownHosts(t *testing.T, address string) {
	t.Helper()

	var serverKey ssh.PublicKey

	config := &ssh.ClientConfig{
		User: "stowmark",
		HostKeyCallback: func(
			_ string,
			_ net.Addr,
			key ssh.PublicKey,
		) error {
			serverKey = key

			return nil
		},
		Timeout: 10 * time.Second,
	}

	connection, err := ssh.Dial("tcp", address, config)
	if connection != nil {
		_ = connection.Close()
	}
	require.Error(t, err)
	require.NotNil(t, serverKey)

	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")

	knownHostsEntry := knownhosts.Line(
		[]string{knownhosts.Normalize(address)},
		serverKey,
	)

	require.NoError(
		t,
		os.WriteFile(
			knownHostsPath,
			[]byte(knownHostsEntry+"\n"),
			0o600,
		),
	)

	t.Setenv("STOWMARK_SSH_KNOWN_HOSTS", knownHostsPath)
}
