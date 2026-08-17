package object

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/model"
)

type Encoder struct {
	handlersFactory *compression.HandlersFactory
}

func (e Encoder) Encode(ctx context.Context, filePath, hash string, offset, size int64, comp *repository.Compression, source *os.File) (bytes.Buffer, error) {
	if err := ctx.Err(); err != nil {
		return bytes.Buffer{}, err
	}
	section := io.NewSectionReader(
		source,
		offset,
		size,
	)

	var encoded bytes.Buffer

	handler, err := e.handlersFactory.GetHandler(
		comp.CompType(),
	)
	if err != nil {
		return bytes.Buffer{}, err
	}

	encoder, err := handler.Encode(
		&encoded,
		comp.Level(),
	)
	if err != nil {
		return bytes.Buffer{}, fmt.Errorf("create compression encoder for chunk %q: %w", hash, err)
	}

	_, copyErr := io.Copy(
		encoder.Writer,
		model.ContextReader{
			Ctx:    ctx,
			Reader: section,
		},
	)
	if copyErr != nil {
		if encoder.Closer != nil {
			_ = encoder.Closer()
		}

		return bytes.Buffer{}, fmt.Errorf("compress chunk %q from %q at offset %d: %w", hash, filePath, offset, copyErr)
	}

	if encoder.Closer != nil {
		if err := encoder.Closer(); err != nil {
			return bytes.Buffer{}, fmt.Errorf("finish compression for chunk %q: %w", hash, err)
		}
	}

	calculatedHashBytes := sha256.Sum256(
		encoded.Bytes(),
	)

	calculatedHash := hex.EncodeToString(
		calculatedHashBytes[:],
	)

	if calculatedHash != hash {
		return bytes.Buffer{}, fmt.Errorf("chunk hash mismatch: expected %s, calculated %s", hash, calculatedHash)
	}
	return encoded, nil
}

func NewEncoder(handlersFactory *compression.HandlersFactory) *Encoder {
	return &Encoder{handlersFactory: handlersFactory}
}
