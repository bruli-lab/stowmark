package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

const ListQueryName = "snapshot-list"

type SnapshotListQuery struct {
	RepositoryPath string
	PrivateKeyPath *string
}

func (l SnapshotListQuery) Name() string {
	return ListQueryName
}

type SnapshotList struct {
	manifestRepo ManifestRepository
	keys         keyring
}

func (l *SnapshotList) Handle(ctx context.Context, query cqs.Query) (any, error) {
	q, ok := query.(SnapshotListQuery)
	if !ok {
		return nil, cqs.NewInvalidQueryError(ListQueryName, query.Name())
	}
	codec, err := l.keys.forRepository(ctx, q.RepositoryPath, q.PrivateKeyPath)
	if err != nil {
		return nil, err
	}
	manifests, err := l.manifestRepo.List(ctx, codec)
	if err != nil {
		return nil, err
	}
	resumes := make([]snapshot.ManifestResume, len(manifests))
	for i := range manifests {
		resumes[i] = *manifests[i].Resume()
	}
	return resumes, nil
}

func NewSnapshotList(
	folderRepo FolderRepository,
	manifestRepo ManifestRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
) *SnapshotList {
	return &SnapshotList{manifestRepo: manifestRepo, keys: newKeyring(folderRepo, asymmetricRepo, symmetricRepo)}
}
