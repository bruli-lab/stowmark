package snapshot

import "context"

type Listing struct {
	manifestRepo ManifestRepository
}

func (l Listing) List(ctx context.Context) ([]ManifestResume, error) {
	manifests, err := l.manifestRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	resume := make([]ManifestResume, len(manifests))
	for i, man := range manifests {
		var (
			files int
			size  int64
		)
		for _, f := range man.Files() {
			files++
			size += f.Size()
		}
		resume[i] = *NewManifestResume(man.Id(), man.CreatedAt(), files, size, man.Source())
	}
	return resume, nil
}

func NewListing(manifestRepo ManifestRepository) *Listing {
	return &Listing{manifestRepo: manifestRepo}
}
