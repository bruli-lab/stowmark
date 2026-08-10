package repository

import (
	"fmt"
	"net/url"
)

var ErrInvalidRepositoryPath = fmt.Errorf("invalid repository path")

type RepositoryType string

const (
	Local  RepositoryType = "local"
	Ssh    RepositoryType = "ssh"
	Smb    RepositoryType = "smb"
	S3     RepositoryType = "s3"
	WebDAV RepositoryType = "webdav"
)

func ParseRepositoryType(raw string) (RepositoryType, error) {
	u, err := url.Parse(raw)
	if err != nil || raw == "" {
		return "", ErrInvalidRepositoryPath
	}
	switch u.Scheme {
	case "ssh", "sftp":
		return Ssh, nil
	case "smb":
		return Smb, nil
	case "s3":
		return S3, nil
	case "http", "https", "webdav", "webdavs":
		return WebDAV, nil
	default:
		return Local, nil
	}
}
