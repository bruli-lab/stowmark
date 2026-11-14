package repositories

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/blob"
)

type Repositories interface {
	FolderRepository() app.FolderRepository
	RepositoryPath() string
	BlobStore() (blob.Store, error)
	ObjectRepository() (app.ObjectRepository, error)
	ManifestRepository() (app.ManifestRepository, error)
	Close() error
}

type backend interface {
	FolderRepository() app.FolderRepository
	RepositoryPath() string
	BlobStore() (blob.Store, error)
	Close() error
}

type blobRepositories struct {
	backend
}

func (b blobRepositories) ObjectRepository() (app.ObjectRepository, error) {
	store, err := b.BlobStore()
	if err != nil {
		return nil, err
	}
	return blob.NewObjectRepository(store), nil
}

func (b blobRepositories) ManifestRepository() (app.ManifestRepository, error) {
	store, err := b.BlobStore()
	if err != nil {
		return nil, err
	}
	return blob.NewManifestRepository(store), nil
}

func NewHandler(ctx context.Context, value string) (Repositories, error) {
	selected, err := newBackend(ctx, value)
	if err != nil {
		return nil, err
	}
	return blobRepositories{backend: selected}, nil
}

func newBackend(ctx context.Context, value string) (backend, error) {
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
	return nil, repository.ErrInvalidRepositoryPath
}
