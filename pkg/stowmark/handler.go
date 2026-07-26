package stowmark

import (
	"github.com/bruli-lab/stowmark.git/internal/domain/repository"
	"github.com/bruli-lab/stowmark.git/internal/infra/disk"
)

type Handler struct {
	folderRepository repository.FolderRepository
}

func NewHandler(repositoryPath string) (*Handler, error) {
	_, err := repository.ParseRepositoryType(repositoryPath)
	if err != nil {
		return nil, err
	}

	folderRepo := disk.NewFolderRepositoryRepository()
	return &Handler{folderRepository: folderRepo}, nil
}
