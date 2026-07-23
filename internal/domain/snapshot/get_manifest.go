package snapshot

import "context"

type GetManifest struct {
	manifestRepo ManifestRepository
}

func (g GetManifest) Get(ctx context.Context, snapshotID string) (*Manifest, error) {
	return g.manifestRepo.Get(ctx, snapshotID)
}

func NewGetManifest(manifestRepo ManifestRepository) *GetManifest {
	return &GetManifest{manifestRepo: manifestRepo}
}
