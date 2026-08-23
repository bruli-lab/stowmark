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

func (l SnapshotList) Handle(ctx context.Context, _ cqs.Query) (any, error) {
	return l.svc.List(ctx)
}

func NewSnapshotList(svc *snapshot.Listing) *SnapshotList {
	return &SnapshotList{svc: svc}
}
