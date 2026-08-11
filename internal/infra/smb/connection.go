package smb

import (
	"context"
	"fmt"
	"net"

	"github.com/hirochachacha/go-smb2"
)

type Connection struct {
	conn    net.Conn
	session *smb2.Session
	Share   *smb2.Share
}

func Connect(
	ctx context.Context,
	address string,
	username string,
	password string,
	shareName string,
) (*Connection, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("connect to SMB server %q: %w", address, err)
	}

	dialer := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     username,
			Password: password,
		},
	}

	session, err := dialer.Dial(conn)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("create SMB session: %w", err)
	}

	share, err := session.Mount(shareName)
	if err != nil {
		_ = session.Logoff()
		_ = conn.Close()
		return nil, fmt.Errorf("mount SMB share %q: %w", shareName, err)
	}

	return &Connection{
		conn:    conn,
		session: session,
		Share:   share,
	}, nil
}

func (c *Connection) Close() error {
	if c.Share != nil {
		_ = c.Share.Umount()
	}
	if c.session != nil {
		_ = c.session.Logoff()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
