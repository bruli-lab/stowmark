package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
)

type Handler struct {
	folderRepository   repository.FolderRepository
	sourceRepository   snapshot.SourceRepository
	manifestRepository snapshot.ManifestRepository
	objectRepository   snapshot.ObjectRepository
	repositoryPath     string
}

func NewHandler(ctx context.Context, repositoryPath string) (*Handler, error) {
	repoHandler, err := repositories.NewHandler(ctx, repositoryPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = repoHandler.Close()
	}()

	folderRepo := repoHandler.FolderRepository()
	sourceRepo := disk.NewSourceRepository()
	manifestRepo, err := repoHandler.ManifestRepository()
	if err != nil {
		return nil, err
	}
	objectRepo, err := repoHandler.ObjectRepository()
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
