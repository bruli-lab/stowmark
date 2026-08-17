package object

import (
	"errors"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

func GetHashes(obj *snapshot.File) ([]string, error) {
	if obj == nil {
		return nil, errors.New("snapshot object is required")
	}

	chunks := obj.Chunks()
	if len(chunks) > 0 {
		hashes := make([]string, 0, len(chunks))

		for index, chunk := range chunks {
			hash := chunk.Hash()
			if len(hash) < 3 {
				return nil, fmt.Errorf(
					"invalid hash for chunk %d of %q: %q",
					index+1,
					obj.Path(),
					hash,
				)
			}

			hashes = append(hashes, hash)
		}

		return hashes, nil
	}

	hash := obj.Hash()
	if len(hash) < 3 {
		return nil, fmt.Errorf(
			"invalid object hash for %q: %q",
			obj.Path(),
			hash,
		)
	}

	return []string{hash}, nil
}
