package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
)

const SnapshotGetQueryName = "manifest-get"

type ManifestGetQuery struct {
	RepositoryPath string
	PrivateKeyPath *string
	SnapshotID     string
}

func (s ManifestGetQuery) Name() string {
	return SnapshotGetQueryName
}

type ManifestGet struct {
	manifestRepo ManifestRepository
	keys         keyring
}

func (m *ManifestGet) Handle(ctx context.Context, query cqs.Query) (any, error) {
	q, ok := query.(ManifestGetQuery)
	if !ok {
		return nil, cqs.NewInvalidQueryError(SnapshotGetQueryName, query.Name())
	}
	codec, err := m.keys.forRepository(ctx, q.RepositoryPath, q.PrivateKeyPath)
	if err != nil {
		return nil, err
	}
	return m.manifestRepo.Get(ctx, q.SnapshotID, codec)
}

func NewManifestGet(
	folderRepo FolderRepository,
	manifestRepo ManifestRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
) *ManifestGet {
	return &ManifestGet{manifestRepo: manifestRepo, keys: newKeyring(folderRepo, asymmetricRepo, symmetricRepo)}
}
