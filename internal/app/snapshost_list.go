package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

const ListQueryName = "snapshot-list"

type SnaphotListQuery struct{}

func (l SnaphotListQuery) Name() string {
	return ListQueryName
}

type SnapshotList struct {
	svc *snapshot.Listing
}

func (l SnapshotList) Handle(ctx context.Context, query cqs.Query) (any, error) {
	if _, ok := query.(SnaphotListQuery); !ok {
		return nil, cqs.NewInvalidQueryError(ListQueryName, query.Name())
	}
	return l.svc.List(ctx)
}

func NewSnapshotList(svc *snapshot.Listing) *SnapshotList {
	return &SnapshotList{svc: svc}
}
