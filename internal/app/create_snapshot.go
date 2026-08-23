package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/google/uuid"
)

const CreateSnapshotCommandName = "create-snapshot"

type CreateSnapshotCommand struct {
	RepositoryPath string
	SourcePath     string
	PrivateKey     *string
}

func (c CreateSnapshotCommand) Name() string {
	return CreateSnapshotCommandName
}

type CreateSnapshot struct {
	svc *snapshot.Create
}

func (c CreateSnapshot) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(CreateSnapshotCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(CreateSnapshotCommandName, cmd.Name())
	}
	result, err := c.svc.Do(ctx, co.RepositoryPath, co.SourcePath, co.PrivateKey)
	if err != nil {
		return nil, err
	}
	return []event.Event{
		NewCreateSnapshotEvent(result.Id(), result.FileCount(), result.TotalSize()),
	}, nil
}

func NewCreateSnapshot(svc *snapshot.Create) *CreateSnapshot {
	return &CreateSnapshot{svc: svc}
}

type CreateSnapshotEvent struct {
	event.BasicEvent
	SnapshotID string
	FileCount  int
	TotalSize  int64
}

func NewCreateSnapshotEvent(
	snapshotID string,
	fileCount int,
	totalSize int64,
) *CreateSnapshotEvent {
	return &CreateSnapshotEvent{
		BasicEvent: event.NewBasicEvent(CreateSnapshotCommandName, uuid.New(), snapshotID),
		SnapshotID: snapshotID,
		FileCount:  fileCount,
		TotalSize:  totalSize,
	}
}
