package cli

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

type Repositories interface {
	FolderRepository() repository.FolderRepository
	RepositoryPath() string
	ObjectRepository() (snapshot.ObjectRepository, error)
	ManifestRepository() (snapshot.ManifestRepository, error)
	Close() error
}

func NewRepositoriesHandler(ctx context.Context, value string) (Repositories, error) {
	repoType, err := repository.ParseRepositoryType(value)
	if err != nil {
		return nil, err
	}
	switch repoType {
	case repository.Local:
		return NewLocalRepositories(value), nil
	case repository.Ssh:
		return NewSSHRepositories(value)
	case repository.Smb:
		return NewSmbRepositories(ctx, value)
	case repository.S3:
		return NewS3Repositories(ctx, value)
	case repository.WebDAV:
		return NewWebDavRepositories(value)
	case repository.Gcs:
		return NewGCSRepositories(ctx, value)
	}
	return nil, nil
}
