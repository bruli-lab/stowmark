package object

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/model"
)

const maxCompressionNameSize = 32

var (
	ErrMetadataKeyRequired   = errors.New("the metadata key is required to identify encrypted objects")
	ErrInvalidObjectEncoding = errors.New("invalid object encoding")
)

func NewContentHasher(codec repository.Codec) (hash.Hash, error) {
	if !codec.EncryptsData() {
		return sha256.New(), nil
	}
	if len(codec.MetadataKey()) == 0 {
		return nil, ErrMetadataKeyRequired
	}
	key, err := encrypt.ObjectIDKey(codec.MetadataKey())
	if err != nil {
		return nil, err
	}
	return hmac.New(sha256.New, key), nil
}

func Digest(ctx context.Context, source io.Reader, codec repository.Codec, factory *compression.HandlersFactory) (string, error) {
	if codec.ContentAddressed() {
		hasher, err := NewContentHasher(codec)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(hasher, model.ContextReader{Ctx: ctx, Reader: source}); err != nil {
			return "", fmt.Errorf("hash data: %w", err)
		}
		return hex.EncodeToString(hasher.Sum(nil)), nil
	}

	comp := codec.Compression()
	if comp == nil {
		return "", ErrCompressionRequired
	}
	handler, err := factory.GetHandler(comp.CompType())
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	if err := compress(ctx, hasher, source, comp, handler); err != nil {
		return "", fmt.Errorf("compress data for hashing: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func headerError(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return fmt.Errorf("%w: the object header is truncated", ErrInvalidObjectEncoding)
	}
	return fmt.Errorf("read object header: %w", err)
}

func writeCompressionHeader(destination io.Writer, compType repository.CompressionType) error {
	name := compType.String()
	if name == "" || len(name) > maxCompressionNameSize {
		return fmt.Errorf("%w: compression name %q", ErrInvalidObjectEncoding, name)
	}
	if _, err := destination.Write(append([]byte{byte(len(name))}, name...)); err != nil {
		return fmt.Errorf("write object header: %w", err)
	}
	return nil
}

func readCompressionHeader(source io.Reader) (repository.CompressionType, error) {
	size := make([]byte, 1)
	if _, err := io.ReadFull(source, size); err != nil {
		return "", headerError(err)
	}
	if size[0] == 0 || size[0] > maxCompressionNameSize {
		return "", fmt.Errorf("%w: compression name size %d", ErrInvalidObjectEncoding, size[0])
	}
	name := make([]byte, size[0])
	if _, err := io.ReadFull(source, name); err != nil {
		return "", headerError(err)
	}
	compType, err := repository.ParseCompressionType(string(name))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidObjectEncoding, err)
	}
	return *compType, nil
}
