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
	Progress        Progress
}

func (r RestoreSnapshotCommand) Name() string {
	return RestoreSnapshotCommandName
}

type RestoreSnapshot struct {
	manifestRepo ManifestRepository
	restorer     restorer
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
	codec, err := r.keys.forRepository(ctx, co.RepositoryPath, co.PrivateKeyPath)
	if err != nil {
		return nil, err
	}
	man, err := r.manifestRepo.Get(ctx, co.SnapshotID, codec)
	if err != nil {
		return nil, err
	}
	result, err := r.restorer.restoreSnapshot(ctx, man, codec, co.DestinationPath, co.Progress)
	if err != nil {
		return nil, err
	}
	return result.Restored(), nil
}

func NewRestoreSnapshot(
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	destination RestoreDestination,
	folderRepo FolderRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
) *RestoreSnapshot {
	return NewRestoreSnapshotWithWorkers(manifestRepo, objectRepo, destination, folderRepo, asymmetricRepo, symmetricRepo, locker, defaultRestoreWorkers)
}

func NewRestoreSnapshotWithWorkers(
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	destination RestoreDestination,
	folderRepo FolderRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
	workers int,
) *RestoreSnapshot {
	return &RestoreSnapshot{
		manifestRepo: manifestRepo,
		restorer:     newRestorer(objectRepo, destination, workers),
		keys:         newKeyring(folderRepo, asymmetricRepo, symmetricRepo),
		locker:       locker,
	}
}
