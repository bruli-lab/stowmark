package ssh

import (
	"fmt"

	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
)

type Client struct {
	ssh  *gossh.Client
	sftp *sftp.Client
}

func (c *Client) Sftp() *sftp.Client {
	return c.sftp
}

func (c *Client) Close() error {
	sftpErr := c.sftp.Close()
	sshErr := c.ssh.Close()

	if sftpErr != nil {
		return fmt.Errorf("close SFTP client: %w", sftpErr)
	}

	if sshErr != nil {
		return fmt.Errorf("close SSH client: %w", sshErr)
	}

	return nil
}

func NewClient(address string, config *gossh.ClientConfig) (*Client, error) {
	sshClient, err := gossh.Dial("tcp", address, config)
	if err != nil {
		return nil, fmt.Errorf(
			"connect to SSH server %q: %w",
			address,
			err,
		)
	}

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		_ = sshClient.Close()

		return nil, fmt.Errorf("create SFTP client: %w", err)
	}

	return &Client{
		ssh:  sshClient,
		sftp: sftpClient,
	}, nil
}
