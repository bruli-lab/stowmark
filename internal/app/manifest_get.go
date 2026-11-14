package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
)

const SnapshotGetQueryName = "manifest-get"

type ManifestGetQuery struct {
	SnapshotID string
}

func (s ManifestGetQuery) Name() string {
	return SnapshotGetQueryName
}

type ManifestGet struct {
	manifestRepo ManifestRepository
}

func (m *ManifestGet) Handle(ctx context.Context, query cqs.Query) (any, error) {
	q, ok := query.(ManifestGetQuery)
	if !ok {
		return nil, cqs.NewInvalidQueryError(SnapshotGetQueryName, query.Name())
	}
	return m.manifestRepo.Get(ctx, q.SnapshotID)
}

func NewManifestGet(manifestRepo ManifestRepository) *ManifestGet {
	return &ManifestGet{manifestRepo: manifestRepo}
}
