package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/google/uuid"
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
}

func (r *RestoreSnapshot) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(RestoreSnapshotCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(RestoreSnapshotCommandName, cmd.Name())
	}
	result, err := r.restore(ctx, co)
	if err != nil {
		return nil, err
	}
	failed := make([]Failed, len(result.Failed()))
	for i, f := range result.Failed() {
		failed[i] = Failed{
			Path:   f.Path(),
			Reason: f.Reason(),
		}
	}
	return []event.Event{
		NewRestoreSnapshotEvent(result.SnapshotID(), result.TotalFiles(), failed, result.IsSuccess()),
	}, nil
}

func (r *RestoreSnapshot) restore(ctx context.Context, co RestoreSnapshotCommand) (*snapshot.Result, error) {
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
	return result, nil
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
) *RestoreSnapshot {
	return &RestoreSnapshot{
		manifestRepo: manifestRepo,
		objectRepo:   objectRepo,
		keys:         newKeyring(folderRepo, asymmetricRepo, symmetricRepo),
	}
}

type Failed struct {
	Path, Reason string
}

type RestoreSnapshotEvent struct {
	event.BasicEvent
	SnapshotID  string
	TotalFiles  int
	FailedFiles []Failed
	IsSuccess   bool
}

func NewRestoreSnapshotEvent(snapshotID string, totalFiles int, failed []Failed, isSuccess bool) *RestoreSnapshotEvent {
	return &RestoreSnapshotEvent{
		BasicEvent:  event.NewBasicEvent(RestoreSnapshotCommandName, uuid.New(), snapshotID),
		SnapshotID:  snapshotID,
		TotalFiles:  totalFiles,
		FailedFiles: failed,
		IsSuccess:   isSuccess,
	}
}
