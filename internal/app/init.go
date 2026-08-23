package app

import (
	"context"
	"time"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/google/uuid"
)

const InitCommandName = "init"

type InitCommand struct {
	Repository *repository.Repository
	Force      bool
}

func (i InitCommand) Name() string {
	return InitCommandName
}

type Init struct {
	svc *repository.Init
}

func (i Init) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(InitCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(InitCommandName, cmd.Name())
	}
	result, err := i.svc.Do(ctx, co.Repository, co.Force)
	if err != nil {
		return nil, err
	}
	return []event.Event{
		NewInitEvent(result.ID, result.Warnings),
	}, nil
}

func NewInit(svc *repository.Init) *Init {
	return &Init{svc: svc}
}

type InitEvent struct {
	event.BasicEvent
	RepositoryID string
	Warnings     []string
}

func NewInitEvent(repositoryID string, warnings []string) *InitEvent {
	return &InitEvent{
		BasicEvent:   event.NewBasicEvent("init", uuid.New(), repositoryID),
		RepositoryID: repositoryID,
		Warnings:     warnings,
	}
}

func (i *InitEvent) EventID() uuid.UUID {
	return i.IDAttr
}

func (i *InitEvent) EventName() string {
	return i.NameAttr
}

func (i *InitEvent) EventAt() time.Time {
	return i.AtAttr
}

func (i *InitEvent) AggregateRootID() string {
	return i.AggregateRootIDAttr
}
