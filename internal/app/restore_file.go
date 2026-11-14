package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
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
}

func (r *RestoreFile) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(RestoreFileCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(RestoreFileCommandName, cmd.Name())
	}
	key, err := r.keys.forRepository(ctx, co.RepositoryPath, co.PrivateKeyPath)
	if err != nil {
		return nil, err
	}
	man, err := r.manifestRepo.Get(ctx, co.SnapshotID)
	if err != nil {
		return nil, err
	}
	file, err := man.FindFile(co.FilePath)
	if err != nil {
		return nil, err
	}
	if err := restoreFile(ctx, r.objectRepo, man, file, co.DestinationPath, key); err != nil {
		return nil, err
	}
	return []event.Event{
		NewRestoreSnapshotEvent(co.SnapshotID, 1, nil, true),
	}, nil
}

func NewRestoreFile(
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	folderRepo FolderRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
) *RestoreFile {
	return &RestoreFile{
		manifestRepo: manifestRepo,
		objectRepo:   objectRepo,
		keys:         newKeyring(folderRepo, asymmetricRepo, symmetricRepo),
	}
}
