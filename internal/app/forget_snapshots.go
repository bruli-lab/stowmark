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
	PrivateKeyPath *string
	Policy         *snapshot.RetentionPolicy
}

func (f ForgetSnapshotsCommand) Name() string {
	return ForgetSnapshotsCommandName
}

type ForgetSnapshots struct {
	manifestRepo ManifestRepository
	keys         keyring
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
		codec, err := f.keys.forRepository(ctx, co.RepositoryPath, co.PrivateKeyPath)
		if err != nil {
			return err
		}
		decisions, err := applyRetention(ctx, f.manifestRepo, codec, co.Policy)
		if err != nil {
			return err
		}
		for i := range decisions {
			if decisions[i].Keep() {
				continue
			}
			forgotten := decisions[i].Forgotten()
			err := f.manifestRepo.Delete(ctx, forgotten.SnapshotID())
			if until, retained := repository.RetainedUntil(err); retained {
				events = append(events, decisions[i].ForgetPostponed(until))
				continue
			}
			if err != nil {
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

func applyRetention(ctx context.Context, manifestRepo ManifestRepository, codec repository.Codec, policy *snapshot.RetentionPolicy) ([]snapshot.RetentionDecision, error) {
	manifests, err := manifestRepo.List(ctx, codec)
	if err != nil {
		return nil, err
	}

	resumes := make([]snapshot.ManifestResume, len(manifests))
	for i := range manifests {
		resumes[i] = *manifests[i].Resume()
	}

	return policy.Apply(resumes, time.Now()), nil
}

func NewForgetSnapshots(
	folderRepo FolderRepository,
	manifestRepo ManifestRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
) *ForgetSnapshots {
	return &ForgetSnapshots{manifestRepo: manifestRepo, keys: newKeyring(folderRepo, asymmetricRepo, symmetricRepo), locker: locker}
}
