package repositories

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/bruli-lab/stowmark/internal/config"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var ErrSSHPassphraseRequired = errors.New("the SSH private key is protected with a passphrase: set STOWMARK_SSH_PRIVATE_KEY_PASSPHRASE or load the key into an ssh-agent")

type noopCloser struct{}

func (noopCloser) Close() error { return nil }

func NewSSHAuth(ctx context.Context, conf *config.SSHConfig) (gossh.AuthMethod, io.Closer, error) {
	if conf.UsesAgent() {
		return agentAuth(ctx, conf.AgentSocket)
	}
	signer, err := privateKeySigner(conf.PrivateKey, conf.PrivateKeyPassphrase)
	if err != nil {
		return nil, nil, err
	}
	return gossh.PublicKeys(signer), noopCloser{}, nil
}

func privateKeySigner(path, passphrase string) (gossh.Signer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read SSH private key: %w", err)
	}
	var signer gossh.Signer
	if passphrase == "" {
		signer, err = gossh.ParsePrivateKey(data)
	} else {
		signer, err = gossh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
	}
	var missing *gossh.PassphraseMissingError
	if errors.As(err, &missing) {
		return nil, ErrSSHPassphraseRequired
	}
	if err != nil {
		return nil, fmt.Errorf("parse SSH private key: %w", err)
	}
	return signer, nil
}

func agentAuth(ctx context.Context, socket string) (gossh.AuthMethod, io.Closer, error) {
	var dialer net.Dialer
	connection, err := dialer.DialContext(ctx, "unix", socket)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to ssh-agent %q: %w", socket, err)
	}
	return gossh.PublicKeysCallback(agent.NewClient(connection).Signers), connection, nil
}
