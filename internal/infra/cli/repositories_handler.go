package cli

import (
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

type Repositories interface {
	FolderRepository() repository.FolderRepository
	RepositoryPath() string
	ObjectRepository() (snapshot.ObjectRepository, error)
	ManifestRepository() (snapshot.ManifestRepository, error)
}

func NewRepositoriesHandler(value string) (Repositories, error) {
	repoType, err := repository.ParseRepositoryType(value)
	if err != nil {
		return nil, err
	}
	switch repoType {
	case repository.Local:
		return NewLocalRepositories(value), nil
	case repository.Ssh:
		return NewSSHRepositories(value)
	}
	return nil, nil
}
