package snapshot

import (
	"context"
	"errors"
	"fmt"
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
		obj, err := v.objectRepo.ReadObject(
			ctx,
			f.Path(),
			f.Hash(),
		)
		if err != nil {
			if errors.As(err, &NotFoundError{}) {
				result.AddFailed(*NewFailedResult(
					f.Path(),
					err.Error(),
				))
				continue
			}

			return nil, err
		}

		if obj.Hash() != f.Hash() {
			result.AddFailed(*NewFailedResult(
				f.Path(),
				fmt.Sprintf(
					"hash mismatch: expected %s, calculated %s",
					f.Hash(),
					obj.Hash(),
				),
			))
			continue
		}

		result.AddSuccess()
	}

	return result, nil
}

func NewVerifier(objectRepo ObjectRepository, manifestRepo ManifestRepository) *Verifier {
	return &Verifier{objectRepo: objectRepo, manifestRepo: manifestRepo}
}
