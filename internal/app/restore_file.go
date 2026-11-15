package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
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
	restorer     restorer
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
		snapshot.NewFileRestored(co.SnapshotID, co.FilePath),
	}, nil
}

func (r *RestoreFile) restore(ctx context.Context, co RestoreFileCommand) error {
	codec, err := r.keys.forRepository(ctx, co.RepositoryPath, co.PrivateKeyPath)
	if err != nil {
		return err
	}
	man, err := r.manifestRepo.Get(ctx, co.SnapshotID, codec)
	if err != nil {
		return err
	}
	file, err := man.FindFile(co.FilePath)
	if err != nil {
		return err
	}
	return r.restorer.restoreEntry(ctx, man, *file, codec, co.DestinationPath)
}

func NewRestoreFile(
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	destination RestoreDestination,
	folderRepo FolderRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
) *RestoreFile {
	return &RestoreFile{
		manifestRepo: manifestRepo,
		restorer:     newRestorer(objectRepo, destination, 1),
		keys:         newKeyring(folderRepo, asymmetricRepo, symmetricRepo),
		locker:       locker,
	}
}
