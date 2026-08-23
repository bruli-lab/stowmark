package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

const VerifySnapshotQueryName = "verify-snapshot"

type VerifySnapshotQuery struct {
	SnapshotID     string
	RepositoryPath string
	PrivateKeyPath *string
}

func (v VerifySnapshotQuery) Name() string {
	return VerifySnapshotQueryName
}

type VerifySnapshot struct {
	svc *snapshot.Verifier
}

func (v VerifySnapshot) Handle(ctx context.Context, query cqs.Query) (any, error) {
	q, ok := query.(VerifySnapshotQuery)
	if !ok {
		return nil, cqs.NewInvalidQueryError(VerifySnapshotQueryName, query.Name())
	}
	return v.svc.Verify(ctx, q.RepositoryPath, q.SnapshotID, q.PrivateKeyPath)
}

func NewVerifySnapshot(svc *snapshot.Verifier) *VerifySnapshot {
	return &VerifySnapshot{svc: svc}
}
