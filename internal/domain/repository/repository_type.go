package repository

import (
	"fmt"
	"net/url"
)

var ErrInvalidRepositoryPath = fmt.Errorf("invalid repository path")

type RepositoryType string

const (
	RepositoryLocal  RepositoryType = "local"
	RepositorySSH    RepositoryType = "ssh"
	RepositorySMB    RepositoryType = "smb"
	RepositoryS3     RepositoryType = "s3"
	RepositoryWebDAV RepositoryType = "webdav"
)

func ParseRepositoryType(raw string) (RepositoryType, error) {
	u, err := url.Parse(raw)
	if err != nil || raw == "" {
		return "", ErrInvalidRepositoryPath
	}
	switch u.Scheme {
	case "ssh", "sftp":
		return RepositorySSH, nil
	case "smb":
		return RepositorySMB, nil
	case "s3":
		return RepositoryS3, nil
	case "http", "https", "webdav", "webdavs":
		return RepositoryWebDAV, nil
	default:
		return RepositoryLocal, nil
	}
}
