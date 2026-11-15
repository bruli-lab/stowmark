package repositories_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type sshTestKey struct {
	private ed25519.PrivateKey
	public  gossh.PublicKey
}

func newSSHTestKey(t *testing.T) sshTestKey {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sshPublic, err := gossh.NewPublicKey(public)
	require.NoError(t, err)
	return sshTestKey{private: private, public: sshPublic}
}

func (k sshTestKey) write(t *testing.T, passphrase string) string {
	t.Helper()
	var (
		block *pem.Block
		err   error
	)
	if passphrase == "" {
		block, err = gossh.MarshalPrivateKey(k.private, "")
	} else {
		block, err = gossh.MarshalPrivateKeyWithPassphrase(k.private, "", []byte(passphrase))
	}
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(block), 0o600))
	return path
}

func startSSHServer(t *testing.T, authorized gossh.PublicKey) string {
	t.Helper()
	hostKey := newSSHTestKey(t)
	hostSigner, err := gossh.NewSignerFromKey(hostKey.private)
	require.NoError(t, err)
	serverConfig := &gossh.ServerConfig{
		PublicKeyCallback: func(_ gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			if bytes.Equal(key.Marshal(), authorized.Marshal()) {
				return &gossh.Permissions{}, nil
			}
			return nil, errors.New("unauthorized key")
		},
	}
	serverConfig.AddHostKey(hostSigner)

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = connection.Close() }()
				_, channels, requests, err := gossh.NewServerConn(connection, serverConfig)
				if err != nil {
					return
				}
				go gossh.DiscardRequests(requests)
				for channel := range channels {
					_ = channel.Reject(gossh.Prohibited, "test server")
				}
			}()
		}
	}()
	return listener.Addr().String()
}

func startAgent(t *testing.T, keys ...sshTestKey) string {
	t.Helper()
	keyring := agent.NewKeyring()
	for _, key := range keys {
		require.NoError(t, keyring.Add(agent.AddedKey{PrivateKey: key.private}))
	}
	directory, err := os.MkdirTemp("", "agent")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	socket := filepath.Join(directory, "agent.sock")
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "unix", socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = connection.Close() }()
				_ = agent.ServeAgent(keyring, connection)
			}()
		}
	}()
	return socket
}

func dialWith(t *testing.T, address string, conf *config.SSHConfig) error {
	t.Helper()
	auth, closer, err := repositories.NewSSHAuth(t.Context(), conf)
	if err != nil {
		return err
	}
	defer func() { require.NoError(t, closer.Close()) }()
	client, err := gossh.Dial("tcp", address, &gossh.ClientConfig{
		User:            "stowmark",
		Auth:            []gossh.AuthMethod{auth},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec // in-process test server
		Timeout:         5 * time.Second,
	})
	if err != nil {
		return err
	}
	return client.Close()
}

func TestNewSSHAuth(t *testing.T) {
	t.Parallel()

	key := newSSHTestKey(t)
	other := newSSHTestKey(t)
	address := startSSHServer(t, key.public)

	tests := []struct {
		name        string
		conf        func(t *testing.T) *config.SSHConfig
		expectedErr error
		dialFails   bool
	}{
		{
			name: "with an unencrypted private key, then it authenticates",
			conf: func(t *testing.T) *config.SSHConfig {
				return &config.SSHConfig{PrivateKey: key.write(t, "")}
			},
		},
		{
			name: "with an encrypted private key and its passphrase, then it authenticates",
			conf: func(t *testing.T) *config.SSHConfig {
				return &config.SSHConfig{PrivateKey: key.write(t, "secret"), PrivateKeyPassphrase: "secret"}
			},
		},
		{
			name: "with an encrypted private key and no passphrase, then it returns a passphrase required error",
			conf: func(t *testing.T) *config.SSHConfig {
				return &config.SSHConfig{PrivateKey: key.write(t, "secret")}
			},
			expectedErr: repositories.ErrSSHPassphraseRequired,
		},
		{
			name: "with an encrypted private key and a wrong passphrase, then it returns an error",
			conf: func(t *testing.T) *config.SSHConfig {
				return &config.SSHConfig{PrivateKey: key.write(t, "secret"), PrivateKeyPassphrase: "wrong"}
			},
			expectedErr: x509.IncorrectPasswordError,
		},
		{
			name: "with a private key and an agent, then it uses only the private key",
			conf: func(t *testing.T) *config.SSHConfig {
				return &config.SSHConfig{PrivateKey: other.write(t, ""), AgentSocket: startAgent(t, key)}
			},
			dialFails: true,
		},
		{
			name: "with only an ssh-agent that holds the key, then it authenticates through the agent",
			conf: func(t *testing.T) *config.SSHConfig {
				return &config.SSHConfig{AgentSocket: startAgent(t, other, key)}
			},
		},
		{
			name: "with an ssh-agent that is not running, then it returns an error",
			conf: func(t *testing.T) *config.SSHConfig {
				return &config.SSHConfig{AgentSocket: filepath.Join(t.TempDir(), "missing.sock")}
			},
			dialFails: true,
		},
	}
	for _, tt := range tests {
		t.Run(`Given an SSH server that authorizes one key,
		when the client authenticates `+tt.name, func(t *testing.T) {
			t.Parallel()

			err := dialWith(t, address, tt.conf(t))

			switch {
			case tt.expectedErr != nil:
				require.ErrorIs(t, err, tt.expectedErr)
			case tt.dialFails:
				require.Error(t, err)
			default:
				require.NoError(t, err)
			}
		})
	}
}
