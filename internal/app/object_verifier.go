package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

type objectVerifier struct {
	objectRepo ObjectRepository
}

func (o objectVerifier) verify(ctx context.Context, expectedHash string, key objectKey) (string, error) {
	if expectedHash == "" {
		return "object hash is empty", nil
	}
	reader, err := o.objectRepo.ReadObject(ctx, expectedHash, key.symmetricKey, key.generation)
	if err != nil {
		if errors.As(err, &snapshot.NotFoundError{}) || errors.Is(err, snapshot.ErrCorruptObject) {
			return err.Error(), nil
		}
		return "", err
	}

	hasher := sha256.New()
	_, copyErr := io.Copy(hasher, &contextReader{ctx: ctx, reader: reader})
	closeErr := reader.Close()

	if copyErr != nil {
		return "", fmt.Errorf("read object %q: %w", expectedHash, copyErr)
	}

	if closeErr != nil {
		return "", fmt.Errorf("close object %q: %w", expectedHash, closeErr)
	}

	calculatedHash := hex.EncodeToString(hasher.Sum(nil))
	if calculatedHash != expectedHash {
		return fmt.Sprintf("hash mismatch: expected %s, calculated %s", expectedHash, calculatedHash), nil
	}

	return "", nil
}
