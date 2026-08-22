package object

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/model"
)

type Encoder struct {
	handlersFactory   *compression.HandlersFactory
	encryptionHandler *encrypt.AESGCMHandler
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

	if comp == nil {
		return bytes.Buffer{}, errors.New("compression configuration is required")
	}

	section := io.NewSectionReader(source, offset, size)

	var compressed bytes.Buffer

	handler, err := e.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return bytes.Buffer{}, err
	}

	encoder, err := handler.Encode(&compressed, comp.Level())
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

		return bytes.Buffer{}, fmt.Errorf(
			"compress chunk %q from %q at offset %d: %w",
			hash,
			filePath,
			offset,
			copyErr,
		)
	}

	if encoder.Closer != nil {
		if err := encoder.Closer(); err != nil {
			return bytes.Buffer{}, fmt.Errorf(
				"finish compression for chunk %q: %w",
				hash,
				err,
			)
		}
	}

	if err := validateHash(compressed.Bytes(), hash); err != nil {
		return bytes.Buffer{}, err
	}

	if len(key) == 0 {
		return compressed, nil
	}

	encrypted, err := e.encrypt(ctx, compressed.Bytes(), key)
	if err != nil {
		return bytes.Buffer{}, fmt.Errorf("encrypt chunk %q: %w", hash, err)
	}

	return encrypted, nil
}

func (e Encoder) encrypt(ctx context.Context, data, key []byte) (bytes.Buffer, error) {
	var encrypted bytes.Buffer

	encoder, err := e.encryptionHandler.Encode(&encrypted, key)
	if err != nil {
		return bytes.Buffer{}, fmt.Errorf("create encryption encoder: %w", err)
	}

	_, copyErr := io.Copy(
		encoder.Writer,
		model.ContextReader{
			Ctx:    ctx,
			Reader: bytes.NewReader(data),
		},
	)
	if copyErr != nil {
		if encoder.Closer != nil {
			_ = encoder.Closer()
		}

		return bytes.Buffer{}, fmt.Errorf("write encryption input: %w", copyErr)
	}

	if encoder.Closer != nil {
		if err := encoder.Closer(); err != nil {
			return bytes.Buffer{}, fmt.Errorf("finish encryption: %w", err)
		}
	}

	return encrypted, nil
}

func NewEncoder(handlersFactory *compression.HandlersFactory) *Encoder {
	return &Encoder{
		handlersFactory:   handlersFactory,
		encryptionHandler: encrypt.NewAESGCMHandler(),
	}
}

func validateHash(data []byte, expectedHash string) error {
	calculatedHashBytes := sha256.Sum256(data)
	calculatedHash := hex.EncodeToString(calculatedHashBytes[:])

	if calculatedHash != expectedHash {
		return fmt.Errorf("chunk hash mismatch: expected %s, calculated %s", expectedHash, calculatedHash)
	}

	return nil
}
