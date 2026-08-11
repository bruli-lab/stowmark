package cli

import (
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
)

type LocalRepositories struct {
	repositoryPath string
}

func (l LocalRepositories) ManifestRepository() (snapshot.ManifestRepository, error) {
	return disk.NewManifestRepository(l.repositoryPath)
}

func (l LocalRepositories) ObjectRepository() (snapshot.ObjectRepository, error) {
	return disk.NewObjectRepository(l.repositoryPath)
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
