package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
)

type Handler struct {
	folderRepository           app.FolderRepository
	sourceRepository           app.SourceRepository
	manifestRepository         app.ManifestRepository
	objectRepository           app.ObjectRepository
	repositoryPath             string
	repositoriesHandler        repositories.Repositories
	asymmetricKeyPaiRepository app.AsymmetricKeyPairRepository
	symmetricKeyRepository     app.SymmetricKeyRepository
	lockRepository             app.LockRepository
	locker                     *app.RepositoryLocker
}

func (h *Handler) Close() error {
	return h.repositoriesHandler.Close()
}

func NewHandler(ctx context.Context, repositoryPath string) (*Handler, error) {
	repoHandler, err := repositories.NewHandler(ctx, repositoryPath)
	if err != nil {
		return nil, err
	}

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
	lockRepo, err := repoHandler.LockRepository()
	if err != nil {
		return nil, err
	}

	return &Handler{
		folderRepository:           folderRepo,
		sourceRepository:           sourceRepo,
		manifestRepository:         manifestRepo,
		objectRepository:           objectRepo,
		repositoryPath:             repoHandler.RepositoryPath(),
		repositoriesHandler:        repoHandler,
		asymmetricKeyPaiRepository: disk.NewAsymmetricKeyPairRepository(),
		symmetricKeyRepository:     encrypt.NewSymmetricRepository(),
		lockRepository:             lockRepo,
		locker:                     app.NewRepositoryLocker(lockRepo),
	}, nil
}
