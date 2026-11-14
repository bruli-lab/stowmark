package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/google/uuid"
)

const ForgetSnapshotsCommandName = "forget-snapshots"

var ErrMissingRetentionPolicy = errors.New("retention policy is required")

type ForgetSnapshotsCommand struct {
	RepositoryPath string
	Policy         *snapshot.RetentionPolicy
	DryRun         bool
}

func (f ForgetSnapshotsCommand) Name() string {
	return ForgetSnapshotsCommandName
}

type ForgetSnapshots struct {
	manifestRepo ManifestRepository
	locker       *RepositoryLocker
}

func (f *ForgetSnapshots) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(ForgetSnapshotsCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(ForgetSnapshotsCommandName, cmd.Name())
	}
	if co.Policy == nil {
		return nil, ErrMissingRetentionPolicy
	}

	var decisions []snapshot.RetentionDecision
	err := f.locker.WithLock(ctx, lockKindFor(co.DryRun), func(ctx context.Context) error {
		applied, err := f.forget(ctx, co)
		decisions = applied
		return err
	})
	if err != nil {
		return nil, err
	}

	return []event.Event{NewForgetSnapshotsEvent(co.RepositoryPath, decisions, co.DryRun)}, nil
}

func (f *ForgetSnapshots) forget(ctx context.Context, co ForgetSnapshotsCommand) ([]snapshot.RetentionDecision, error) {
	manifests, err := f.manifestRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	resumes := make([]snapshot.ManifestResume, len(manifests))
	for i := range manifests {
		resumes[i] = *manifests[i].Resume()
	}

	decisions := co.Policy.Apply(resumes, time.Now())
	if co.DryRun {
		return decisions, nil
	}

	for _, decision := range decisions {
		if decision.Keep() {
			continue
		}
		if err := f.manifestRepo.Delete(ctx, decision.Snapshot().ID()); err != nil {
			return nil, fmt.Errorf("forget snapshot %s: %w", decision.Snapshot().ID(), err)
		}
	}

	return decisions, nil
}

func lockKindFor(dryRun bool) repository.LockKind {
	if dryRun {
		return repository.SharedLock
	}
	return repository.ExclusiveLock
}

func NewForgetSnapshots(manifestRepo ManifestRepository, locker *RepositoryLocker) *ForgetSnapshots {
	return &ForgetSnapshots{manifestRepo: manifestRepo, locker: locker}
}

type ForgetSnapshotsEvent struct {
	event.BasicEvent
	Decisions []snapshot.RetentionDecision
	DryRun    bool
}

func (f *ForgetSnapshotsEvent) Forgotten() int {
	forgotten := 0
	for _, decision := range f.Decisions {
		if !decision.Keep() {
			forgotten++
		}
	}
	return forgotten
}

func NewForgetSnapshotsEvent(repositoryPath string, decisions []snapshot.RetentionDecision, dryRun bool) *ForgetSnapshotsEvent {
	return &ForgetSnapshotsEvent{
		BasicEvent: event.NewBasicEvent(ForgetSnapshotsCommandName, uuid.New(), repositoryPath),
		Decisions:  decisions,
		DryRun:     dryRun,
	}
}
