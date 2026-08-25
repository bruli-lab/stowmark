package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
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
	svc *snapshot.RestoreFile
}

func (r RestoreFile) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(RestoreFileCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(RestoreFileCommandName, cmd.Name())
	}
	if err := r.svc.Restore(ctx, co.SnapshotID, co.FilePath, co.RepositoryPath, co.DestinationPath, co.PrivateKeyPath); err != nil {
		return nil, err
	}
	return []event.Event{
		NewRestoreSnapshotEvent(co.SnapshotID, 1, nil, true),
	}, nil
}

func NewRestoreFile(svc *snapshot.RestoreFile) *RestoreFile {
	return &RestoreFile{svc: svc}
}
