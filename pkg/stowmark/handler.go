package stowmark

import (
	"github.com/bruli-lab/stowmark.git/internal/domain/repository"
	"github.com/bruli-lab/stowmark.git/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark.git/internal/infra/disk"
)

type Handler struct {
	folderRepository   repository.FolderRepository
	sourceRepository   snapshot.SourceRepository
	manifestRepository snapshot.ManifestRepository
	objectRepository   snapshot.ObjectRepository
	repositoryPath     string
}

func NewHandler(repositoryPath string) (*Handler, error) {
	_, err := repository.ParseRepositoryType(repositoryPath)
	if err != nil {
		return nil, err
	}

	folderRepo := disk.NewFolderRepositoryRepository()
	sourceRepo := disk.NewSourceRepository()
	manifestRepo, err := disk.NewManifestRepository(repositoryPath)
	if err != nil {
		return nil, err
	}
	objectRepo, err := disk.NewObjectRepository(repositoryPath)
	if err != nil {
		return nil, err
	}

	return &Handler{
		folderRepository:   folderRepo,
		sourceRepository:   sourceRepo,
		manifestRepository: manifestRepo,
		objectRepository:   objectRepo,
		repositoryPath:     repositoryPath,
	}, nil
}
