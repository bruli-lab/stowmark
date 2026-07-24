package snapshot

import (
	"context"
	"errors"
)

type Verifier struct {
	objectRepo   ObjectRepository
	manifestRepo ManifestRepository
}

func (v Verifier) Verify(ctx context.Context, snapshotID string) (*VerifyResult, error) {
	manifest, err := v.manifestRepo.Get(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	result := NewVerifyResult(snapshotID)
	for _, f := range manifest.Files() {
		obj, err := v.objectRepo.ReadObject(ctx, f.Path(), f.Hash())
		if err != nil {
			switch {
			case errors.As(err, &NotFoundError{}):
				result.AddFailed(*NewFailedCheck(f.Path(), err.Error()))
				continue
			default:
				return nil, err
			}
		}
		var failed bool
		if obj.Hash() != f.Hash() {
			result.AddFailed(*NewFailedCheck(f.Path(), "hash mismatch"))
			failed = true
		}
		if obj.Size() != f.Size() {
			result.AddFailed(*NewFailedCheck(f.Path(), "size mismatch"))
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
