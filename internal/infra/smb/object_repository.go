package smb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"sort"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/chunkio"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/bruli-lab/stowmark/internal/infra/object"
	"github.com/cloudsoda/go-smb2"
)

type ObjectRepository struct {
	repositoryPath    string
	share             *smb2.Share
	handlersFactory   *compression.HandlersFactory
	encryptionHandler *encrypt.AESGCMHandler
	pipeline          *object.Pipeline
}

func (o ObjectRepository) ListEncryptedObjects(ctx context.Context, generation uint64) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	directory := object.EncryptedGenerationPath(
		o.repositoryPath,
		generation,
		true,
	)

	prefixDirectories, err := o.share.ReadDir(directory)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}

		return nil, fmt.Errorf("list encrypted generation %d in SMB directory %q: %w", generation, directory, err)
	}

	var hashes []string

	for _, prefixDirectory := range prefixDirectories {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if !prefixDirectory.IsDir() ||
			len(prefixDirectory.Name()) != 2 {
			continue
		}

		prefix := prefixDirectory.Name()
		prefixPath := path.Join(directory, prefix)

		entries, err := o.share.ReadDir(prefixPath)
		if err != nil {
			return nil, fmt.Errorf("list encrypted object directory %q: %w", prefixPath, err)
		}

		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			if entry.IsDir() {
				continue
			}

			if entry.Name() == "" || isTemporaryFile(entry.Name()) {
				continue
			}

			hashes = append(
				hashes,
				prefix+entry.Name(),
			)
		}
	}

	sort.Strings(hashes)

	return hashes, nil
}

func (o ObjectRepository) ReadEncryptedObject(ctx context.Context, hash string, generation uint64, key []byte) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(hash) < 3 {
		return nil, fmt.Errorf("invalid object hash %q", hash)
	}

	if len(key) == 0 {
		return nil, errors.New("symmetric key is required")
	}

	directory := object.EncryptedGenerationPath(o.repositoryPath, generation, true)

	objectPath := path.Join(
		directory,
		hash[:2],
		hash[2:],
	)

	source, err := o.share.Open(objectPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("encrypted object %q from generation %d: %w", hash, generation, os.ErrNotExist)
		}

		return nil, fmt.Errorf("open encrypted SMB object %q: %w", objectPath, err)
	}

	if err := ctx.Err(); err != nil {
		_ = source.Close()

		return nil, err
	}

	decoded, err := o.encryptionHandler.Decode(source, key)
	if err != nil {
		_ = source.Close()

		return nil, fmt.Errorf("decrypt object %q from generation %d: %w", hash, generation, err)
	}

	return &model.ReadCloser{
		Reader: decoded,
		Closer: source,
	}, nil
}

func (o ObjectRepository) SaveRekeyedObject(ctx context.Context, hash string, source io.Reader, generation uint64, key []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(hash) < 3 {
		return fmt.Errorf("invalid object hash %q", hash)
	}

	if source == nil {
		return errors.New("source is required")
	}

	if len(key) == 0 {
		return errors.New("symmetric key is required")
	}

	directory := object.EncryptedGenerationPath(o.repositoryPath, generation, true)

	prefixDirectory := path.Join(directory, hash[:2])

	if err := o.share.MkdirAll(prefixDirectory, 0o755); err != nil {
		return fmt.Errorf("create SMB encrypted object directory %q: %w", prefixDirectory, err)
	}

	objectPath := path.Join(prefixDirectory, hash[2:])

	destination, err := o.share.OpenFile(
		objectPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		0o644,
	)
	if err != nil {
		return fmt.Errorf("create rekeyed SMB object %q: %w", objectPath, err)
	}

	abortSave := func() {
		_ = destination.Close()
		_ = o.share.Remove(objectPath)
	}

	encoder, err := o.encryptionHandler.Encode(destination, key)
	if err != nil {
		abortSave()

		return fmt.Errorf("create encryption encoder for object %q: %w", hash, err)
	}

	if _, err := io.Copy(encoder.Writer, source); err != nil {
		_ = encoder.Closer()
		abortSave()

		return fmt.Errorf("encrypt rekeyed object %q: %w", hash, err)
	}

	if err := ctx.Err(); err != nil {
		_ = encoder.Closer()
		abortSave()

		return err
	}

	if err := encoder.Closer(); err != nil {
		abortSave()

		return fmt.Errorf("finalize encryption of rekeyed object %q: %w", hash, err)
	}

	if err := destination.Close(); err != nil {
		_ = o.share.Remove(objectPath)

		return fmt.Errorf("close rekeyed SMB object %q: %w", objectPath, err)
	}

	return nil
}

func (o ObjectRepository) DeleteEncryptedGeneration(ctx context.Context, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	directory := object.EncryptedGenerationPath(o.repositoryPath, generation, true)

	slog.WarnContext(
		ctx,
		"SMB backend cannot delete the encrypted generation automatically; remove the directory manually",
		slog.Uint64("generation", generation),
		slog.String("directory", directory),
	)

	return nil
}

func (o ObjectRepository) AbortRekey(ctx context.Context, generation uint64) error {
	if err := o.DeleteEncryptedGeneration(ctx, generation); err != nil {
		return fmt.Errorf("abort rekey failed for generation %d: %w", generation, err)
	}
	return nil
}

func (o ObjectRepository) ReadObject(ctx context.Context, hash string, symmetricKey []byte, generation uint64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(hash) < 3 {
		return nil, fmt.Errorf("invalid object hash %q", hash)
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, true)
	objectPath := dest.ObjectPath
	reader, err := o.share.Open(objectPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) ||
			os.IsNotExist(err) {
			return nil, snapshot.NewNotFoundError(hash)
		}

		return nil, fmt.Errorf("open SMB object %q: %w", objectPath, err)
	}

	if len(symmetricKey) == 0 {
		return reader, nil
	}
	decoded, err := o.encryptionHandler.Decode(reader, symmetricKey)
	if err != nil {
		_ = reader.Close()
		return nil, fmt.Errorf("decrypt object %q: %w", dest.ObjectPath, err)
	}

	return &model.ReadCloser{
		Reader: decoded,
		Closer: reader,
	}, nil
}

func (o ObjectRepository) SaveChunk(ctx context.Context, filePath, hash string, offset, size int64, comp *repository.Compression, key []byte, generation uint64) error {
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

	section := io.NewSectionReader(source, offset, size)
	if err := o.writeObject(ctx, section, hash, comp, key, generation); err != nil {
		return fmt.Errorf("write chunk from %q at offset %d: %w", filePath, offset, err)
	}

	return nil
}

func (o ObjectRepository) Save(ctx context.Context, filePath, hash string, comp *repository.Compression, symmetricKey []byte, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(hash) < 3 {
		return fmt.Errorf("invalid object hash %q", hash)
	}

	source, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", filePath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	return o.writeObject(ctx, source, hash, comp, symmetricKey, generation)
}

func (o ObjectRepository) writeObject(ctx context.Context, source io.Reader, hash string, comp *repository.Compression, key []byte, generation uint64) error {
	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, key, true)
	share := o.share.WithContext(ctx)
	if err := share.MkdirAll(dest.DirectoryPath, 0o755); err != nil {
		return fmt.Errorf("create SMB object directory %q: %w", dest.DirectoryPath, err)
	}

	return writeFileAtomically(share, dest.ObjectPath, func(destination io.Writer) error {
		return o.pipeline.Write(ctx, destination, source, comp, key, hash)
	})
}

func (o ObjectRepository) AlreadyExists(ctx context.Context, hash string, symmetricKey []byte, generation uint64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if len(hash) < 3 {
		return false, fmt.Errorf("invalid object hash %q", hash)
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, true)
	destinationPath := dest.ObjectPath
	_, err := o.share.Stat(destinationPath)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("check destination file %q: %w", destinationPath, err)
	}
}

func (o ObjectRepository) RestoreObject(ctx context.Context, comp *repository.Compression, obj *snapshot.File, symmetricKey []byte, generation uint64) error {
	return object.RestoreFile(ctx, comp, obj, func(ctx context.Context, hash string, destination io.Writer) (int64, error) {
		return o.restoreObjectPart(ctx, comp, hash, destination, symmetricKey, generation)
	})
}

func (o ObjectRepository) restoreObjectPart(ctx context.Context, comp *repository.Compression, hash string, destination io.Writer, symmetricKey []byte, generation uint64) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	if len(hash) < 3 {
		return 0, fmt.Errorf("invalid object hash %q", hash)
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, true)
	sourcePath := dest.ObjectPath

	source, err := o.share.Open(sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, snapshot.NewNotFoundError(hash)
		}

		return 0, fmt.Errorf("open SMB object %q: %w", sourcePath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	written, err := o.pipeline.Read(ctx, destination, source, comp, symmetricKey, hash)
	if err != nil {
		return written, fmt.Errorf("restore object %q: %w", sourcePath, err)
	}

	return written, nil
}

func NewObjectRepository(repositoryPath string, share *smb2.Share) *ObjectRepository {
	handlersFactory := compression.NewHandlersFactory()
	return &ObjectRepository{
		repositoryPath:    repositoryPath,
		share:             share,
		handlersFactory:   handlersFactory,
		encryptionHandler: encrypt.NewAESGCMHandler(),
		pipeline:          object.NewPipeline(handlersFactory),
	}
}
