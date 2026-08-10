package cli

import (
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
)

type LocalRepositories struct {
	repositoryPath string
}

func (l LocalRepositories) RepositoryPath() string {
	return l.repositoryPath
}

func (l LocalRepositories) FolderRepository() repository.FolderRepository {
	return disk.NewFolderRepositoryRepository()
}

func NewLocalRepositories(value string) *LocalRepositories {
	return &LocalRepositories{repositoryPath: value}
}

