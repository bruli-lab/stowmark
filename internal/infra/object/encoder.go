package object

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
)

type Encoder struct {
	pipeline *Pipeline
}

func (e Encoder) Encode(
	ctx context.Context,
	filePath, hash string,
	offset, size int64,
	comp *repository.Compression,
	key []byte,
	source *os.File,
) (bytes.Buffer, error) {
	if err := ctx.Err(); err != nil {
		return bytes.Buffer{}, err
	}

	var encoded bytes.Buffer
	section := io.NewSectionReader(source, offset, size)
	if err := e.pipeline.Write(ctx, &encoded, section, comp, key, hash); err != nil {
		return bytes.Buffer{}, fmt.Errorf("encode chunk %q from %q at offset %d: %w", hash, filePath, offset, err)
	}

	return encoded, nil
}

func NewEncoder(handlersFactory *compression.HandlersFactory) *Encoder {
	return &Encoder{pipeline: NewPipeline(handlersFactory)}
}
