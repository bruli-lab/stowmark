package snapshot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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

	for _, file := range manifest.Files() {
		reason, err := v.verifyFile(ctx, &file)
		if err != nil {
			return nil, err
		}
		if reason != "" {
			result.AddFailed(*NewFailedResult(
				file.Path(),
				reason,
			))
			continue
		}

		result.AddSuccess()
	}

	return result, nil
}

func (v Verifier) verifyFile(ctx context.Context, file *File) (string, error) {
	if len(file.Chunks()) == 0 {
		return v.verifyObject(
			ctx,
			file.Hash(),
		)
	}
	chunks := file.Chunks()
	for index, chunk := range chunks {
		reason, err := v.verifyObject(ctx, chunk.Hash())
		if err != nil {
			return "", err
		}

		if reason != "" {
			return fmt.Sprintf("chunk %d/%d: %s", index+1, len(chunks), reason), nil
		}
	}

	return "", nil
}

func (v Verifier) verifyObject(ctx context.Context, expectedHash string) (string, error) {
	if expectedHash == "" {
		return "object hash is empty", nil
	}
	reader, err := v.objectRepo.ReadObject(ctx, expectedHash)
	if err != nil {
		var notFoundErr NotFoundError

		if errors.As(err, &notFoundErr) {
			return err.Error(), nil
		}
		return "", err
	}

	hasher := sha256.New()

	_, copyErr := io.Copy(hasher, &contextReader{
		ctx:    ctx,
		reader: reader,
	})

	closeErr := reader.Close()

	if copyErr != nil {
		return "", fmt.Errorf("read object %q: %w", expectedHash, copyErr)
	}

	if closeErr != nil {
		return "", fmt.Errorf("close object %q: %w", expectedHash, closeErr)
	}

	calculatedHash := hex.EncodeToString(
		hasher.Sum(nil),
	)

	if calculatedHash != expectedHash {
		return fmt.Sprintf("hash mismatch: expected %s, calculated %s", expectedHash, calculatedHash), nil
	}

	return "", nil
}

func NewVerifier(objectRepo ObjectRepository, manifestRepo ManifestRepository) *Verifier {
	return &Verifier{objectRepo: objectRepo, manifestRepo: manifestRepo}
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}

	return r.reader.Read(data)
}
