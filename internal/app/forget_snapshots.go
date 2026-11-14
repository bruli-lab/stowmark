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
)

const ForgetSnapshotsCommandName = "forget-snapshots"

var ErrMissingRetentionPolicy = errors.New("retention policy is required")

type ForgetSnapshotsCommand struct {
	RepositoryPath string
	Policy         *snapshot.RetentionPolicy
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

	events := make([]event.Event, 0)
	err := f.locker.WithLock(ctx, repository.ExclusiveLock, func(ctx context.Context) error {
		decisions, err := applyRetention(ctx, f.manifestRepo, co.Policy)
		if err != nil {
			return err
		}
		for i := range decisions {
			if decisions[i].Keep() {
				continue
			}
			forgotten := decisions[i].Forgotten()
			if err := f.manifestRepo.Delete(ctx, forgotten.SnapshotID()); err != nil {
				return fmt.Errorf("forget snapshot %s: %w", forgotten.SnapshotID(), err)
			}
			events = append(events, forgotten)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return events, nil
}

func applyRetention(ctx context.Context, manifestRepo ManifestRepository, policy *snapshot.RetentionPolicy) ([]snapshot.RetentionDecision, error) {
	manifests, err := manifestRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	resumes := make([]snapshot.ManifestResume, len(manifests))
	for i := range manifests {
		resumes[i] = *manifests[i].Resume()
	}

	return policy.Apply(resumes, time.Now()), nil
}

func NewForgetSnapshots(manifestRepo ManifestRepository, locker *RepositoryLocker) *ForgetSnapshots {
	return &ForgetSnapshots{manifestRepo: manifestRepo, locker: locker}
}
