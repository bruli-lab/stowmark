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
	svc *snapshot.Restore
}

func (r RestoreSnapshot) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(RestoreSnapshotCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(RestoreSnapshotCommandName, cmd.Name())
	}
	result, err := r.svc.Restore(ctx, co.SnapshotID, co.RepositoryPath, co.DestinationPath, co.PrivateKeyPath)
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

func NewRestoreSnapshot(svc *snapshot.Restore) *RestoreSnapshot {
	return &RestoreSnapshot{svc: svc}
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
