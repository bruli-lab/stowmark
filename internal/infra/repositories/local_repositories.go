package repositories

import (
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
)

type LocalRepositories struct {
	repositoryPath string
}

func (l LocalRepositories) Close() error {
	return nil
}

func (l LocalRepositories) RepositoryPath() string {
	return l.repositoryPath
}

func (l LocalRepositories) FolderRepository() repository.FolderRepository {
	return disk.NewFolderRepositoryRepository()
}

func (l LocalRepositories) BlobStore() (blob.Store, error) {
	return disk.NewBlobStore(l.repositoryPath)
}

func NewLocalRepositories(value string) *LocalRepositories {
	return &LocalRepositories{repositoryPath: value}
}
