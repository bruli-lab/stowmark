package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/chunkio"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/bruli-lab/stowmark/internal/infra/object"
	"github.com/pkg/sftp"
)

type ObjectRepository struct {
	client            *sftp.Client
	handlersFactory   *compression.HandlersFactory
	repositoryPath    string
	encoder           *object.Encoder
	encryptionHandler *encrypt.AESGCMHandler
}

func (o ObjectRepository) ListEncryptedObjects(ctx context.Context, generation uint64) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	directory := object.EncryptedGenerationPath(o.repositoryPath, generation, true)

	prefixDirectories, err := o.client.ReadDir(directory)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}

		return nil, fmt.Errorf("list encrypted generation %d in SSH directory %q: %w", generation, directory, err)
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

		entries, err := o.client.ReadDir(prefixPath)
		if err != nil {
			return nil, fmt.Errorf("list encrypted SSH directory %q: %w", prefixPath, err)
		}

		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			if entry.IsDir() || entry.Name() == "" {
				continue
			}

			hashes = append(hashes, prefix+entry.Name())
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
	objectPath := path.Join(directory, hash[:2], hash[2:])

	source, err := o.client.Open(objectPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("encrypted object %q from generation %d: %w", hash, generation, os.ErrNotExist)
		}

		return nil, fmt.Errorf("open encrypted SSH object %q: %w", objectPath, err)
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

	if err := o.client.MkdirAll(prefixDirectory); err != nil {
		return fmt.Errorf("create encrypted SSH directory %q: %w", prefixDirectory, err)
	}

	objectPath := path.Join(prefixDirectory, hash[2:])

	destination, err := o.client.OpenFile(objectPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("create rekeyed SSH object %q: %w", objectPath, err)
	}

	abortSave := func() {
		_ = destination.Close()
		_ = o.client.Remove(objectPath)
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
		_ = o.client.Remove(objectPath)
		return fmt.Errorf("close rekeyed SSH object %q: %w", objectPath, err)
	}

	return nil
}

func (o ObjectRepository) DeleteEncryptedGeneration(ctx context.Context, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	directory := object.EncryptedGenerationPath(o.repositoryPath, generation, true)

	prefixDirectories, err := o.client.ReadDir(directory)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("list encrypted generation %d in SSH directory %q: %w", generation, directory, err)
	}

	for _, prefixDirectory := range prefixDirectories {
		if err := ctx.Err(); err != nil {
			return err
		}

		prefixPath := path.Join(directory, prefixDirectory.Name())

		if !prefixDirectory.IsDir() {
			if err := o.client.Remove(prefixPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("delete encrypted SSH object %q: %w", prefixPath, err)
			}

			continue
		}

		entries, err := o.client.ReadDir(prefixPath)
		if err != nil {
			return fmt.Errorf("list encrypted SSH directory %q: %w", prefixPath, err)
		}

		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}

			objectPath := path.Join(prefixPath, entry.Name())

			if entry.IsDir() {
				return fmt.Errorf("unexpected directory %q inside encrypted generation %d", objectPath, generation)
			}

			if err := o.client.Remove(objectPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("delete encrypted SSH object %q: %w", objectPath, err)
			}
		}

		if err := o.client.RemoveDirectory(prefixPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete encrypted SSH directory %q: %w", prefixPath, err)
		}
	}

	if err := o.client.RemoveDirectory(directory); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete encrypted generation %d from SSH directory %q: %w", generation, directory, err)
	}

	return nil
}

func (o ObjectRepository) AbortRekey(ctx context.Context, generation uint64) error {
	if err := o.DeleteEncryptedGeneration(ctx, generation); err != nil {
		return fmt.Errorf("abort rekey generation %d: %w", generation, err)
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

	reader, err := o.client.Open(objectPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) ||
			os.IsNotExist(err) {
			return nil, snapshot.NewNotFoundError(hash)
		}

		return nil, fmt.Errorf("open SSH object %q: %w", objectPath, err)
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

	encoded, err := o.encoder.Encode(ctx, filePath, hash, offset, size, comp, key, source)
	if err != nil {
		return err
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, key, true)

	if err := o.client.MkdirAll(dest.DirectoryPath); err != nil {
		return fmt.Errorf("create SSH object directory %q: %w", dest.DirectoryPath, err)
	}

	destination, err := o.client.OpenFile(
		dest.ObjectPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
	)
	if err != nil {
		return fmt.Errorf("create SSH object %q: %w", dest.ObjectPath, err)
	}

	writeCompleted := false

	defer func() {
		_ = destination.Close()

		if !writeCompleted {
			_ = o.client.Remove(dest.ObjectPath)
		}
	}()

	written, writeErr := io.Copy(
		destination,
		model.ContextReader{
			Ctx:    ctx,
			Reader: bytes.NewReader(encoded.Bytes()),
		},
	)

	if writeErr != nil {
		return fmt.Errorf("write SSH object %q: %w", dest.ObjectPath, writeErr)
	}

	if written != int64(encoded.Len()) {
		return fmt.Errorf("incomplete SSH object write %q: expected %d bytes, wrote %d", dest.ObjectPath, encoded.Len(), written)
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf("close SSH object %q: %w", dest.ObjectPath, err)
	}

	writeCompleted = true
	return nil
}

func (o ObjectRepository) RestoreObject(ctx context.Context, comp *repository.Compression, obj *snapshot.File, symmetricKey []byte, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if comp == nil {
		return errors.New("compression configuration is required")
	}

	if obj == nil {
		return errors.New("snapshot object is required")
	}

	hashes, err := object.GetHashes(obj)
	if err != nil {
		return err
	}

	destinationPath := obj.Path()

	if err := os.MkdirAll(
		filepath.Dir(destinationPath),
		0o755,
	); err != nil {
		return fmt.Errorf("create local destination directory for %q: %w", destinationPath, err)
	}

	destination, err := os.OpenFile(
		destinationPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		0o644,
	)
	if err != nil {
		return fmt.Errorf("create local restored file %q: %w", destinationPath, err)
	}

	restoreCompleted := false

	defer func() {
		if restoreCompleted {
			return
		}

		_ = destination.Close()
		_ = os.Remove(destinationPath)
	}()

	var restoredSize int64

	for index, hash := range hashes {
		if err := ctx.Err(); err != nil {
			return err
		}

		written, err := o.restoreObjectPart(ctx, comp, hash, destination, symmetricKey, generation)
		if err != nil {
			return fmt.Errorf("restore remote part %d/%d of %q: %w", index+1, len(hashes), destinationPath, err)
		}

		restoredSize += written
	}

	if restoredSize != obj.Size() {
		return fmt.Errorf("restored size mismatch for %q: expected %d, restored %d", destinationPath, obj.Size(), restoredSize)
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf("close local restored file %q: %w", destinationPath, err)
	}

	restoreCompleted = true

	return nil
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

	source, err := o.client.Open(sourcePath)
	if err != nil {
		return 0, fmt.Errorf("open remote object %q: %w", sourcePath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	reader := io.Reader(source)

	if len(symmetricKey) > 0 {
		reader, err = o.encryptionHandler.Decode(source, symmetricKey)
		if err != nil {
			return 0, fmt.Errorf("decrypt object %q: %w", sourcePath, err)
		}
	}

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return 0, fmt.Errorf("get compression handler %q: %w", comp.CompType(), err)
	}

	decoded, err := handler.Decode(reader)
	if err != nil {
		return 0, fmt.Errorf("decode remote object %q using %q: %w", sourcePath, comp.CompType(), err)
	}
	defer decoded.Closer()

	written, err := io.Copy(
		destination,
		model.ContextReader{
			Ctx:    ctx,
			Reader: decoded.Reader,
		},
	)
	if err != nil {
		return written, fmt.Errorf(
			"copy decoded remote object %q: %w",
			sourcePath,
			err,
		)
	}

	return written, nil
}

func (o ObjectRepository) AlreadyExists(ctx context.Context, hash string, symmetricKey []byte, generation uint64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if len(hash) < 3 {
		return false, fmt.Errorf("invalid object hash %q", hash)
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, true)
	destinationPath := dest.DirectoryPath
	_, err := o.client.Stat(destinationPath)
	switch {
	case err == nil:
		return true, nil

	case os.IsNotExist(err):
		return false, nil

	default:
		return false, fmt.Errorf(
			"check remote destination file %q: %w",
			destinationPath,
			err,
		)
	}
}

func (o ObjectRepository) Save(ctx context.Context, filePath, hash string, comp *repository.Compression, symmetricKey []byte, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if comp == nil {
		return errors.New("compression configuration is required")
	}

	if len(hash) < 3 {
		return fmt.Errorf("invalid object hash %q", hash)
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, true)
	destinationDirectory := dest.DirectoryPath
	destinationPath := dest.ObjectPath

	if err := o.client.MkdirAll(destinationDirectory); err != nil {
		return fmt.Errorf("create remote object directory %q: %w", destinationDirectory, err)
	}

	source, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open local source file %q: %w", filePath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	destination, err := o.client.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}

		return fmt.Errorf("create remote object file %q: %w", destinationPath, err)
	}

	writeCompleted := false

	defer func() {
		_ = destination.Close()

		if !writeCompleted {
			_ = o.client.Remove(destinationPath)
		}
	}()

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return fmt.Errorf("get compression handler %q: %w", comp.CompType(), err)
	}

	output := io.Writer(destination)
	var closeEncryption func() error

	if len(symmetricKey) > 0 {
		encrypted, err := o.encryptionHandler.Encode(destination, symmetricKey)
		if err != nil {
			return fmt.Errorf("create encryption writer: %w", err)
		}

		output = encrypted.Writer
		closeEncryption = encrypted.Closer
	}
	encoded, err := handler.Encode(output, comp.Level())
	if err != nil {
		if closeEncryption != nil {
			_ = closeEncryption()
		}
		return fmt.Errorf("encode remote object %q using %q: %w", destinationPath, comp.CompType(), err)
	}

	if _, err := io.Copy(encoded.Writer, model.ContextReader{Ctx: ctx, Reader: source}); err != nil {
		return fmt.Errorf("write local file %q to remote object %q: %w", filePath, destinationPath, err)
	}

	if encoded.Closer != nil {
		if err := encoded.Closer(); err != nil {
			return fmt.Errorf("close %q encoder for remote object %q: %w", comp.CompType(), destinationPath, err)
		}
	}

	if closeEncryption != nil {
		_ = closeEncryption()
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf("close remote object file %q: %w", destinationPath, err)
	}

	writeCompleted = true

	return nil
}

func NewObjectRepository(repositoryPath string, client *sftp.Client) *ObjectRepository {
	handlersFactory := compression.NewHandlersFactory()
	return &ObjectRepository{
		repositoryPath:    repositoryPath,
		handlersFactory:   handlersFactory,
		client:            client,
		encoder:           object.NewEncoder(handlersFactory),
		encryptionHandler: encrypt.NewAESGCMHandler(),
	}
}
