package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

const PrunePlanQueryName = "prune-plan"

type PrunePlanQuery struct {
	RepositoryPath string
}

func (p PrunePlanQuery) Name() string {
	return PrunePlanQueryName
}

type PrunePlan struct {
	planner prunePlanner
	locker  *RepositoryLocker
}

func (p *PrunePlan) Handle(ctx context.Context, query cqs.Query) (any, error) {
	q, ok := query.(PrunePlanQuery)
	if !ok {
		return nil, cqs.NewInvalidQueryError(PrunePlanQueryName, query.Name())
	}

	var plan *snapshot.PrunePlan
	err := p.locker.WithLock(ctx, repository.SharedLock, func(ctx context.Context) error {
		target, err := p.planner.plan(ctx, q.RepositoryPath)
		if err != nil {
			return err
		}
		plan = target.plan
		return nil
	})
	if err != nil {
		return nil, err
	}

	return plan, nil
}

func NewPrunePlan(
	folderRepo FolderRepository,
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	locker *RepositoryLocker,
) *PrunePlan {
	return &PrunePlan{
		planner: prunePlanner{folderRepo: folderRepo, manifestRepo: manifestRepo, objectRepo: objectRepo},
		locker:  locker,
	}
}
