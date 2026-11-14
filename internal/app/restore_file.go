package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

const RestoreFileCommandName = "restore-file"

type RestoreFileCommand struct {
	SnapshotID, FilePath, RepositoryPath string
	DestinationPath, PrivateKeyPath      *string
}

func (r RestoreFileCommand) Name() string {
	return RestoreFileCommandName
}

type RestoreFile struct {
	manifestRepo ManifestRepository
	objectRepo   ObjectRepository
	keys         keyring
	locker       *RepositoryLocker
}

func (r *RestoreFile) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(RestoreFileCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(RestoreFileCommandName, cmd.Name())
	}
	if err := r.locker.WithLock(ctx, repository.SharedLock, func(ctx context.Context) error {
		return r.restore(ctx, co)
	}); err != nil {
		return nil, err
	}
	return []event.Event{
		NewRestoreSnapshotEvent(co.SnapshotID, 1, nil, true),
	}, nil
}

func (r *RestoreFile) restore(ctx context.Context, co RestoreFileCommand) error {
	key, err := r.keys.forRepository(ctx, co.RepositoryPath, co.PrivateKeyPath)
	if err != nil {
		return err
	}
	man, err := r.manifestRepo.Get(ctx, co.SnapshotID)
	if err != nil {
		return err
	}
	file, err := man.FindFile(co.FilePath)
	if err != nil {
		return err
	}
	return restoreFile(ctx, r.objectRepo, man, file, co.DestinationPath, key)
}

func NewRestoreFile(
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	folderRepo FolderRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
) *RestoreFile {
	return &RestoreFile{
		manifestRepo: manifestRepo,
		objectRepo:   objectRepo,
		keys:         newKeyring(folderRepo, asymmetricRepo, symmetricRepo),
		locker:       locker,
	}
}
