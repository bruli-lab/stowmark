package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

const SnapshotGetQueryName = "manifest-get"

type ManifestGetQuery struct {
	SnapshotID string
}

func (s ManifestGetQuery) Name() string {
	return SnapshotGetQueryName
}

type ManifestGet struct {
	svc *snapshot.GetManifest
}

func (m ManifestGet) Handle(ctx context.Context, query cqs.Query) (any, error) {
	q, ok := query.(ManifestGetQuery)
	if !ok {
		return nil, cqs.NewInvalidQueryError(SnapshotGetQueryName, query.Name())
	}
	return m.svc.Get(ctx, q.SnapshotID)
}

func NewManifestGet(svc *snapshot.GetManifest) *ManifestGet {
	return &ManifestGet{svc: svc}
}
