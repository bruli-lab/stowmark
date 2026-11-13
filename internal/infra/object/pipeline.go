package object

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/model"
)

var (
	ErrHashMismatch        = errors.New("object hash mismatch")
	ErrCompressionRequired = errors.New("compression configuration is required")
)

type Pipeline struct {
	handlersFactory   *compression.HandlersFactory
	encryptionHandler *encrypt.AESGCMHandler
}

func (p Pipeline) Write(
	ctx context.Context,
	destination io.Writer,
	source io.Reader,
	comp *repository.Compression,
	key []byte,
	expectedHash string,
) error {
	if comp == nil {
		return ErrCompressionRequired
	}

	handler, err := p.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return err
	}

	output := destination
	var encryption *encrypt.Encoder
	if len(key) > 0 {
		encryption, err = p.encryptionHandler.Encode(destination, key)
		if err != nil {
			return fmt.Errorf("create encryption writer: %w", err)
		}
		output = encryption.Writer
	}

	hasher := sha256.New()
	compressor, err := handler.Encode(io.MultiWriter(output, hasher), comp.Level())
	if err != nil {
		return fmt.Errorf("create %q compression writer: %w", comp.CompType(), err)
	}

	if _, err := io.Copy(compressor.Writer, model.ContextReader{Ctx: ctx, Reader: source}); err != nil {
		_ = compressor.Closer()
		return fmt.Errorf("compress object: %w", err)
	}

	if err := compressor.Closer(); err != nil {
		return fmt.Errorf("finish %q compression: %w", comp.CompType(), err)
	}

	calculatedHash := hex.EncodeToString(hasher.Sum(nil))
	if calculatedHash != expectedHash {
		return fmt.Errorf("%w: expected %s, calculated %s", ErrHashMismatch, expectedHash, calculatedHash)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if encryption == nil {
		return nil
	}

	if err := encryption.Closer(); err != nil {
		return fmt.Errorf("finish encryption: %w", err)
	}

	return nil
}

func (p Pipeline) Read(
	ctx context.Context,
	destination io.Writer,
	source io.Reader,
	comp *repository.Compression,
	key []byte,
	expectedHash string,
) (int64, error) {
	if comp == nil {
		return 0, ErrCompressionRequired
	}

	handler, err := p.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return 0, err
	}

	input := source
	if len(key) > 0 {
		input, err = p.encryptionHandler.Decode(source, key)
		if err != nil {
			return 0, fmt.Errorf("decrypt object: %w", err)
		}
	}

	hasher := sha256.New()
	compressed := io.TeeReader(model.ContextReader{Ctx: ctx, Reader: input}, hasher)

	decoded, err := handler.Decode(compressed)
	if err != nil {
		return 0, fmt.Errorf("create %q decompression reader: %w", comp.CompType(), err)
	}
	defer decoded.Closer()

	written, err := io.Copy(destination, decoded.Reader)
	if err != nil {
		return written, fmt.Errorf("decompress object: %w", err)
	}

	if _, err := io.Copy(io.Discard, compressed); err != nil {
		return written, fmt.Errorf("read remaining object data: %w", err)
	}

	calculatedHash := hex.EncodeToString(hasher.Sum(nil))
	if calculatedHash != expectedHash {
		return written, fmt.Errorf("%w: expected %s, calculated %s", ErrHashMismatch, expectedHash, calculatedHash)
	}

	return written, nil
}

func NewPipeline(handlersFactory *compression.HandlersFactory) *Pipeline {
	return &Pipeline{
		handlersFactory:   handlersFactory,
		encryptionHandler: encrypt.NewAESGCMHandler(),
	}
}
