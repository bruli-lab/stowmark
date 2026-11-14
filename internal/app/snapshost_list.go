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
	manifestRepo ManifestRepository
}

func (l *SnapshotList) Handle(ctx context.Context, query cqs.Query) (any, error) {
	if _, ok := query.(SnaphotListQuery); !ok {
		return nil, cqs.NewInvalidQueryError(ListQueryName, query.Name())
	}
	manifests, err := l.manifestRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	resumes := make([]snapshot.ManifestResume, len(manifests))
	for i := range manifests {
		resumes[i] = *manifests[i].Resume()
	}
	return resumes, nil
}

func NewSnapshotList(manifestRepo ManifestRepository) *SnapshotList {
	return &SnapshotList{manifestRepo: manifestRepo}
}
