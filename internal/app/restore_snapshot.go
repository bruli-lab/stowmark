//nolint:dupl // CQS command handlers intentionally share the same adapter structure.
package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

const RestoreSnapshotCommandName = "restore-snapshot"

type RestoreSnapshotCommand struct {
	SnapshotID      string
	RepositoryPath  string
	PrivateKeyPath  *string
	DestinationPath *string
}

func (r RestoreSnapshotCommand) Name() string {
	return RestoreSnapshotCommandName
}

type RestoreSnapshot struct {
	manifestRepo ManifestRepository
	objectRepo   ObjectRepository
	keys         keyring
	locker       *RepositoryLocker
}

func (r *RestoreSnapshot) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(RestoreSnapshotCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(RestoreSnapshotCommandName, cmd.Name())
	}
	return withLockedEvent(ctx, r.locker, repository.SharedLock, func(ctx context.Context) (*snapshot.SnapshotRestored, error) {
		return r.restore(ctx, co)
	})
}

func (r *RestoreSnapshot) restore(ctx context.Context, co RestoreSnapshotCommand) (*snapshot.SnapshotRestored, error) {
	key, err := r.keys.forRepository(ctx, co.RepositoryPath, co.PrivateKeyPath)
	if err != nil {
		return nil, err
	}
	man, err := r.manifestRepo.Get(ctx, co.SnapshotID)
	if err != nil {
		return nil, err
	}
	result := snapshot.NewResult(co.SnapshotID)
	for _, f := range man.Files() {
		if err := restoreFile(ctx, r.objectRepo, man, &f, co.DestinationPath, key); err != nil {
			result.AddFailed(*snapshot.NewFailedResult(f.Path(), err.Error()))
			continue
		}
		result.AddSuccess()
	}
	return result.Restored(), nil
}

func restoreFile(ctx context.Context, objectRepo ObjectRepository, man *snapshot.Manifest, file *snapshot.File, destinationPath *string, key objectKey) error {
	if destinationPath != nil {
		if err := file.ChangeSourcePath(man.Source(), *destinationPath); err != nil {
			return err
		}
	}
	return objectRepo.RestoreObject(ctx, man.Compression(), file, key.symmetricKey, key.generation)
}

func NewRestoreSnapshot(
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	folderRepo FolderRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
) *RestoreSnapshot {
	return &RestoreSnapshot{
		manifestRepo: manifestRepo,
		objectRepo:   objectRepo,
		keys:         newKeyring(folderRepo, asymmetricRepo, symmetricRepo),
		locker:       locker,
	}
}
