package blob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

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

func (o ObjectRepository) Save(ctx context.Context, filePath, hash string, codec repository.Codec) error {
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

	if err := o.write(ctx, source, hash, codec); err != nil {
		return fmt.Errorf("save object %q from %q: %w", hash, filePath, err)
	}

	return nil
}

func (o ObjectRepository) SaveChunk(ctx context.Context, filePath string, chunk snapshot.Chunk, codec repository.Codec) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	source, err := chunkio.OpenSource(filePath, chunk.Hash(), chunk.Offset(), chunk.Size())
	if err != nil {
		return err
	}
	defer func() {
		_ = source.Close()
	}()

	if err := o.write(ctx, io.NewSectionReader(source, chunk.Offset(), chunk.Size()), chunk.Hash(), codec); err != nil {
		return fmt.Errorf("save chunk %q from %q at offset %d: %w", chunk.Hash(), filePath, chunk.Offset(), err)
	}

	return nil
}

func (o ObjectRepository) write(ctx context.Context, source io.Reader, hash string, codec repository.Codec) error {
	key, err := objectKey(hash, codec.EncryptsData(), codec.Generation())
	if err != nil {
		return err
	}

	return o.store.Put(ctx, key, func(destination io.Writer) error {
		return o.pipeline.Write(ctx, destination, source, codec, hash)
	})
}

func (o ObjectRepository) AlreadyExists(ctx context.Context, hash string, codec repository.Codec) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	key, err := objectKey(hash, codec.EncryptsData(), codec.Generation())
	if err != nil {
		return false, err
	}

	return o.store.Exists(ctx, key)
}

func (o ObjectRepository) VerifyObject(ctx context.Context, hash string, codec repository.Codec) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	reader, err := o.open(ctx, hash, codec.EncryptsData(), codec.Generation())
	if err != nil {
		return err
	}
	defer func() {
		_ = reader.Close()
	}()

	tracked := &trackedReader{reader: reader}
	verifyErr := o.pipeline.Verify(ctx, tracked, codec, hash)
	switch {
	case verifyErr == nil:
		return nil
	case tracked.err != nil:
		return fmt.Errorf("read object %q: %w", hash, tracked.err)
	case ctx.Err() != nil:
		return ctx.Err()
	default:
		return fmt.Errorf("%w: %w", snapshot.ErrCorruptObject, verifyErr)
	}
}

func (o ObjectRepository) RestoreObject(ctx context.Context, file *snapshot.File, codec repository.Codec) error {
	return object.RestoreFile(ctx, file, func(ctx context.Context, hash string, destination io.Writer) (int64, error) {
		reader, err := o.open(ctx, hash, codec.EncryptsData(), codec.Generation())
		if err != nil {
			return 0, err
		}
		defer func() {
			_ = reader.Close()
		}()

		written, err := o.pipeline.Read(ctx, destination, reader, codec, hash)
		if err != nil {
			return written, fmt.Errorf("restore object %q: %w", hash, err)
		}

		return written, nil
	})
}

func (o ObjectRepository) ListObjects(ctx context.Context, encrypted bool, generation uint64) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := objectsPrefix(encrypted, generation)
	keys, err := o.store.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("list objects from %q: %w", prefix, err)
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

func (o ObjectRepository) DeleteObject(ctx context.Context, hash string, encrypted bool, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key, err := objectKey(hash, encrypted, generation)
	if err != nil {
		return err
	}

	if err := o.store.Delete(ctx, key); err != nil {
		return fmt.Errorf("delete object %q: %w", hash, err)
	}

	return nil
}

func (o ObjectRepository) ListEncryptedObjects(ctx context.Context, generation uint64) ([]string, error) {
	hashes, err := o.ListObjects(ctx, true, generation)
	if err != nil {
		return nil, fmt.Errorf("list encrypted objects from generation %d: %w", generation, err)
	}

	return hashes, nil
}

func (o ObjectRepository) ListEncryptedGenerations(ctx context.Context) ([]uint64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := path.Join(repository.ObjectsFolder, repository.EncryptedFolder)
	keys, err := o.store.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("list encrypted generations: %w", err)
	}

	generations := make([]uint64, 0)
	for _, key := range keys {
		relative, ok := strings.CutPrefix(key, prefix+"/")
		if !ok {
			continue
		}
		segment, _, ok := strings.Cut(relative, "/")
		if !ok {
			continue
		}
		generation, err := strconv.ParseUint(segment, 10, 64)
		if err != nil || slices.Contains(generations, generation) {
			continue
		}
		generations = append(generations, generation)
	}
	slices.Sort(generations)

	return generations, nil
}

func (o ObjectRepository) ReadEncryptedObject(ctx context.Context, hash string, codec repository.Codec) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if !codec.EncryptsData() {
		return nil, ErrSymmetricKeyRequired
	}

	reader, err := o.open(ctx, hash, true, codec.Generation())
	if err != nil {
		return nil, fmt.Errorf("read encrypted object from generation %d: %w", codec.Generation(), err)
	}

	if codec.ContentAddressed() {
		decrypted, err := object.DecryptContent(reader, codec, hash)
		if err != nil {
			_ = reader.Close()
			return nil, corruptObjectError(hash, err)
		}
		return &model.ReadCloser{Reader: decrypted, Closer: reader}, nil
	}

	return o.decrypt(reader, hash, codec.DataKey())
}

func (o ObjectRepository) SaveRekeyedObject(ctx context.Context, hash string, source io.Reader, codec repository.Codec) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if source == nil {
		return ErrSourceRequired
	}

	if !codec.EncryptsData() {
		return ErrSymmetricKeyRequired
	}

	key, err := objectKey(hash, true, codec.Generation())
	if err != nil {
		return err
	}

	return o.store.Put(ctx, key, func(destination io.Writer) error {
		if codec.ContentAddressed() {
			return o.reencryptContent(ctx, destination, source, hash, codec)
		}
		return o.reencryptCompressed(ctx, destination, source, hash, codec)
	})
}

func (o ObjectRepository) reencryptContent(ctx context.Context, destination io.Writer, source io.Reader, hash string, codec repository.Codec) error {
	stream, err := object.EncryptContent(destination, codec, hash)
	if err != nil {
		return fmt.Errorf("create encryption stream for object %q: %w", hash, err)
	}
	if _, err := io.Copy(stream, model.ContextReader{Ctx: ctx, Reader: source}); err != nil {
		return fmt.Errorf("encrypt rekeyed object %q: %w", hash, err)
	}
	if err := stream.Close(); err != nil {
		return fmt.Errorf("finish encryption of rekeyed object %q: %w", hash, err)
	}
	return nil
}

func (o ObjectRepository) reencryptCompressed(ctx context.Context, destination io.Writer, source io.Reader, hash string, codec repository.Codec) error {
	encoder, err := o.encryptionHandler.Encode(destination, codec.DataKey())
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
}

func (o ObjectRepository) DeleteTemporaryObjects(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	cleaner, ok := o.store.(TemporaryCleaner)
	if !ok {
		return 0, nil
	}
	removed, err := cleaner.DeleteTemporaries(ctx, repository.ObjectsFolder)
	if err != nil {
		return removed, fmt.Errorf("delete temporary objects: %w", err)
	}
	return removed, nil
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
		return nil, corruptObjectError(hash, err)
	}

	return &model.ReadCloser{Reader: decoded, Closer: reader}, nil
}

func corruptObjectError(hash string, err error) error {
	if errors.Is(err, encrypt.ErrDecryptionFailed) {
		return fmt.Errorf("%w: decrypt object %q: %w", snapshot.ErrCorruptObject, hash, err)
	}
	return fmt.Errorf("decrypt object %q: %w", hash, err)
}

type trackedReader struct {
	reader io.Reader
	err    error
}

func (t *trackedReader) Read(data []byte) (int, error) {
	n, err := t.reader.Read(data)
	if err != nil && !errors.Is(err, io.EOF) {
		t.err = err
	}
	return n, err
}

func NewObjectRepository(store Store) *ObjectRepository {
	return &ObjectRepository{
		store:             store,
		pipeline:          object.NewPipeline(compression.NewHandlersFactory()),
		encryptionHandler: encrypt.NewAESGCMHandler(),
	}
}
