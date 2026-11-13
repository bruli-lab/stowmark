package blob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/chunkio"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/bruli-lab/stowmark/internal/infra/object"
)

var (
	ErrSymmetricKeyRequired = errors.New("symmetric key is required")
	ErrSourceRequired       = errors.New("source is required")
)

type ObjectRepository struct {
	store             Store
	pipeline          *object.Pipeline
	encryptionHandler *encrypt.AESGCMHandler
}

func (o ObjectRepository) Save(ctx context.Context, filePath, hash string, comp *repository.Compression, symmetricKey []byte, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	source, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", filePath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	if err := o.write(ctx, source, hash, comp, symmetricKey, generation); err != nil {
		return fmt.Errorf("save object %q from %q: %w", hash, filePath, err)
	}

	return nil
}

func (o ObjectRepository) SaveChunk(ctx context.Context, filePath, hash string, offset, size int64, comp *repository.Compression, symmetricKey []byte, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	source, err := chunkio.OpenSource(filePath, hash, offset, size, comp)
	if err != nil {
		return err
	}
	defer func() {
		_ = source.Close()
	}()

	if err := o.write(ctx, io.NewSectionReader(source, offset, size), hash, comp, symmetricKey, generation); err != nil {
		return fmt.Errorf("save chunk %q from %q at offset %d: %w", hash, filePath, offset, err)
	}

	return nil
}

func (o ObjectRepository) write(ctx context.Context, source io.Reader, hash string, comp *repository.Compression, symmetricKey []byte, generation uint64) error {
	key, err := objectKey(hash, len(symmetricKey) > 0, generation)
	if err != nil {
		return err
	}

	return o.store.Put(ctx, key, func(destination io.Writer) error {
		return o.pipeline.Write(ctx, destination, source, comp, symmetricKey, hash)
	})
}

func (o ObjectRepository) AlreadyExists(ctx context.Context, hash string, symmetricKey []byte, generation uint64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	key, err := objectKey(hash, len(symmetricKey) > 0, generation)
	if err != nil {
		return false, err
	}

	return o.store.Exists(ctx, key)
}

func (o ObjectRepository) ReadObject(ctx context.Context, hash string, symmetricKey []byte, generation uint64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	reader, err := o.open(ctx, hash, len(symmetricKey) > 0, generation)
	if err != nil {
		return nil, err
	}

	if len(symmetricKey) == 0 {
		return reader, nil
	}

	return o.decrypt(reader, hash, symmetricKey)
}

func (o ObjectRepository) RestoreObject(ctx context.Context, comp *repository.Compression, obj *snapshot.File, symmetricKey []byte, generation uint64) error {
	return object.RestoreFile(ctx, comp, obj, func(ctx context.Context, hash string, destination io.Writer) (int64, error) {
		reader, err := o.open(ctx, hash, len(symmetricKey) > 0, generation)
		if err != nil {
			return 0, err
		}
		defer func() {
			_ = reader.Close()
		}()

		written, err := o.pipeline.Read(ctx, destination, reader, comp, symmetricKey, hash)
		if err != nil {
			return written, fmt.Errorf("restore object %q: %w", hash, err)
		}

		return written, nil
	})
}

func (o ObjectRepository) ListEncryptedObjects(ctx context.Context, generation uint64) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := generationPrefix(generation)
	keys, err := o.store.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("list encrypted objects from generation %d: %w", generation, err)
	}

	hashes := make([]string, 0, len(keys))
	for _, key := range keys {
		if hash, ok := hashFromKey(prefix, key); ok {
			hashes = append(hashes, hash)
		}
	}
	sort.Strings(hashes)

	return hashes, nil
}

func (o ObjectRepository) ReadEncryptedObject(ctx context.Context, hash string, generation uint64, symmetricKey []byte) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(symmetricKey) == 0 {
		return nil, ErrSymmetricKeyRequired
	}

	reader, err := o.open(ctx, hash, true, generation)
	if err != nil {
		return nil, fmt.Errorf("read encrypted object from generation %d: %w", generation, err)
	}

	return o.decrypt(reader, hash, symmetricKey)
}

func (o ObjectRepository) SaveRekeyedObject(ctx context.Context, hash string, source io.Reader, generation uint64, symmetricKey []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if source == nil {
		return ErrSourceRequired
	}

	if len(symmetricKey) == 0 {
		return ErrSymmetricKeyRequired
	}

	key, err := objectKey(hash, true, generation)
	if err != nil {
		return err
	}

	return o.store.Put(ctx, key, func(destination io.Writer) error {
		encoder, err := o.encryptionHandler.Encode(destination, symmetricKey)
		if err != nil {
			return fmt.Errorf("create encryption encoder for object %q: %w", hash, err)
		}

		hasher := sha256.New()
		if _, err := io.Copy(encoder.Writer, io.TeeReader(model.ContextReader{Ctx: ctx, Reader: source}, hasher)); err != nil {
			return fmt.Errorf("encrypt rekeyed object %q: %w", hash, err)
		}

		calculatedHash := hex.EncodeToString(hasher.Sum(nil))
		if calculatedHash != hash {
			return fmt.Errorf("%w: expected %s, calculated %s", object.ErrHashMismatch, hash, calculatedHash)
		}

		if err := encoder.Closer(); err != nil {
			return fmt.Errorf("finish encryption of rekeyed object %q: %w", hash, err)
		}

		return nil
	})
}

func (o ObjectRepository) DeleteEncryptedGeneration(ctx context.Context, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := o.store.DeletePrefix(ctx, generationPrefix(generation)); err != nil {
		return fmt.Errorf("delete encrypted generation %d: %w", generation, err)
	}

	return nil
}

func (o ObjectRepository) AbortRekey(ctx context.Context, generation uint64) error {
	if err := o.DeleteEncryptedGeneration(ctx, generation); err != nil {
		return fmt.Errorf("abort rekey for generation %d: %w", generation, err)
	}

	return nil
}

func (o ObjectRepository) open(ctx context.Context, hash string, encrypted bool, generation uint64) (io.ReadCloser, error) {
	key, err := objectKey(hash, encrypted, generation)
	if err != nil {
		return nil, err
	}

	reader, err := o.store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, snapshot.NewNotFoundError(hash)
		}
		return nil, fmt.Errorf("open object %q: %w", hash, err)
	}

	return reader, nil
}

func (o ObjectRepository) decrypt(reader io.ReadCloser, hash string, symmetricKey []byte) (io.ReadCloser, error) {
	decoded, err := o.encryptionHandler.Decode(reader, symmetricKey)
	if err != nil {
		_ = reader.Close()
		return nil, fmt.Errorf("decrypt object %q: %w", hash, err)
	}

	return &model.ReadCloser{Reader: decoded, Closer: reader}, nil
}

func NewObjectRepository(store Store) *ObjectRepository {
	return &ObjectRepository{
		store:             store,
		pipeline:          object.NewPipeline(compression.NewHandlersFactory()),
		encryptionHandler: encrypt.NewAESGCMHandler(),
	}
}
