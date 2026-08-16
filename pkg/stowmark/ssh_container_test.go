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

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func startSSHContainer(t *testing.T, ctx context.Context, publicKey []byte) testcontainers.Container {
	t.Helper()

	container, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image: "lscr.io/linuxserver/openssh-server:latest",
				ExposedPorts: []string{
					"2222/tcp",
				},
				Env: map[string]string{
					"PUID":            strconv.Itoa(os.Getuid()),
					"PGID":            strconv.Itoa(os.Getuid()),
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
				Mounts: testcontainers.ContainerMounts{
					testcontainers.BindMount(
						t.TempDir(),
						"/repositories",
					),
				},
				WaitingFor: wait.
					ForListeningPort("2222/tcp").
					WithStartupTimeout(60 * time.Second),
			},
			Started: true,
		},
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(
			t,
			testcontainers.TerminateContainer(container),
		)
	})

	return container
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

	homePath := t.TempDir()
	sshPath := filepath.Join(homePath, ".ssh")

	require.NoError(t, os.MkdirAll(sshPath, 0o700))

	knownHostsEntry := knownhosts.Line(
		[]string{knownhosts.Normalize(address)},
		serverKey,
	)

	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(sshPath, "known_hosts"),
			[]byte(knownHostsEntry+"\n"),
			0o600,
		),
	)

	t.Setenv("HOME", homePath)
}
