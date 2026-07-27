package snapshot

import (
	"context"
	"errors"
)

type Verifier struct {
	objectRepo   ObjectRepository
	manifestRepo ManifestRepository
}

func (v Verifier) Verify(ctx context.Context, snapshotID string) (*Result, error) {
	manifest, err := v.manifestRepo.Get(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	result := NewResult(snapshotID)
	for _, f := range manifest.Files() {
		obj, err := v.objectRepo.ReadObject(ctx, manifest.compression, f.Path(), f.Hash())
		if err != nil {
			switch {
			case errors.As(err, &NotFoundError{}):
				result.AddFailed(*NewFailedResult(f.Path(), err.Error()))
				continue
			default:
				return nil, err
			}
		}
		var failed bool
		if obj.Hash() != f.Hash() {
			result.AddFailed(*NewFailedResult(f.Path(), "hash mismatch"))
			failed = true
		}
		if obj.Size() != f.Size() {
			result.AddFailed(*NewFailedResult(f.Path(), "size mismatch"))
			failed = true
		}
		if !failed {
			result.AddSuccess()
		}
	}
	return result, nil
}

func NewVerifier(objectRepo ObjectRepository, manifestRepo ManifestRepository) *Verifier {
	return &Verifier{objectRepo: objectRepo, manifestRepo: manifestRepo}
}
