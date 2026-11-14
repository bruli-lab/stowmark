package repositories

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"

	"github.com/bruli-lab/stowmark/internal/infra/blob"

	"github.com/bruli-lab/stowmark/internal/config"
	sshinfra "github.com/bruli-lab/stowmark/internal/infra/ssh"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const DefaultSSHPort = "22"

type SSHRepositories struct {
	client         *sshinfra.Client
	repositoryPath string
}

func (s *SSHRepositories) RepositoryPath() string {
	return s.repositoryPath
}

func (s *SSHRepositories) FolderRepository() app.FolderRepository {
	return sshinfra.NewFolderRepositoryRepository(s.client.Sftp())
}

func (s *SSHRepositories) Close() error {
	return s.client.Close()
}

func (s *SSHRepositories) BlobStore() (blob.Store, error) {
	return sshinfra.NewBlobStore(s.client.Sftp(), s.repositoryPath), nil
}

func NewSSHRepositories(address string, environment map[string]string) (*SSHRepositories, error) {
	conf, err := config.NewSSHConfigFrom(environment)
	if err != nil {
		return nil, fmt.Errorf("load SSH configuration: %w", err)
	}

	u, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("parse SSH address %q: %w", address, err)
	}

	if u.User == nil || u.User.Username() == "" {
		return nil, fmt.Errorf("SSH username is required")
	}

	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("SSH host is required")
	}

	port := u.Port()
	if port == "" {
		port = DefaultSSHPort
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home directory: %w", err)
	}

	clientConfig, err := newClientConfig(
		u.User.Username(),
		conf.PrivateKey,
		filepath.Join(homeDir, ".ssh", "known_hosts"),
	)
	if err != nil {
		return nil, err
	}

	client, err := sshinfra.NewClient(
		net.JoinHostPort(host, port),
		clientConfig,
	)
	if err != nil {
		return nil, err
	}

	return &SSHRepositories{
		client:         client,
		repositoryPath: u.Path,
	}, nil
}

func newClientConfig(username, privateKey, knownHostsPath string) (*gossh.ClientConfig, error) {
	file, err := os.ReadFile(privateKey)
	if err != nil {
		return nil, fmt.Errorf("read SSH private key: %w", err)
	}

	signer, err := gossh.ParsePrivateKey(file)
	if err != nil {
		return nil, fmt.Errorf("parse SSH private key: %w", err)
	}

	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf(
			"load known_hosts file %q: %w",
			knownHostsPath,
			err,
		)
	}

	return &gossh.ClientConfig{
		User: username,
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}, nil
}
