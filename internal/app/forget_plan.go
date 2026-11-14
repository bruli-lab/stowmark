package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

const ForgetPlanQueryName = "forget-plan"

type ForgetPlanQuery struct {
	Policy *snapshot.RetentionPolicy
}

func (f ForgetPlanQuery) Name() string {
	return ForgetPlanQueryName
}

type ForgetPlan struct {
	manifestRepo ManifestRepository
	locker       *RepositoryLocker
}

func (f *ForgetPlan) Handle(ctx context.Context, query cqs.Query) (any, error) {
	q, ok := query.(ForgetPlanQuery)
	if !ok {
		return nil, cqs.NewInvalidQueryError(ForgetPlanQueryName, query.Name())
	}
	if q.Policy == nil {
		return nil, ErrMissingRetentionPolicy
	}

	var decisions []snapshot.RetentionDecision
	err := f.locker.WithLock(ctx, repository.SharedLock, func(ctx context.Context) error {
		applied, err := applyRetention(ctx, f.manifestRepo, q.Policy)
		decisions = applied
		return err
	})
	if err != nil {
		return nil, err
	}

	return decisions, nil
}

func NewForgetPlan(manifestRepo ManifestRepository, locker *RepositoryLocker) *ForgetPlan {
	return &ForgetPlan{manifestRepo: manifestRepo, locker: locker}
}
