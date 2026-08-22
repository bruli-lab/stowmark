package disk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/chunkio"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/bruli-lab/stowmark/internal/infra/object"
)

type ObjectRepository struct {
	repositoryPath    string
	handlersFactory   *compression.HandlersFactory
	encryptionHandler *encrypt.AESGCMHandler
}

func (o ObjectRepository) ListEncryptedObjects(ctx context.Context, generation uint64) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	generationPath := object.EncryptedGenerationPath(o.repositoryPath, generation, false)

	hashes := make([]string, 0)

	err := filepath.WalkDir(generationPath, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}

		relativePath, err := filepath.Rel(generationPath, filePath)
		if err != nil {
			return fmt.Errorf("get relative encrypted object path %q: %w", filePath, err)
		}

		parts := strings.Split(filepath.ToSlash(relativePath), "/")
		if len(parts) != 2 || len(parts[0]) != 2 || parts[1] == "" {
			return fmt.Errorf("invalid encrypted object path %q", filePath)
		}

		hashes = append(hashes, parts[0]+parts[1])

		return nil
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return hashes, nil
		}

		return nil, fmt.Errorf("list encrypted objects from generation %d: %w", generation, err)
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
	directory := object.EncryptedGenerationPath(o.repositoryPath, generation, false)

	objectPath := filepath.Join(directory, hash[:2], hash[2:])

	source, err := os.Open(objectPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("encrypted object %q not found: %w", hash, err)
		}

		return nil, fmt.Errorf("open encrypted object %q: %w", objectPath, err)
	}

	decoded, err := o.encryptionHandler.Decode(source, key)
	if err != nil {
		_ = source.Close()

		return nil, fmt.Errorf("decrypt object %q: %w", objectPath, err)
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

	generationPath := object.EncryptedGenerationPath(o.repositoryPath, generation, false)
	directoryPath := filepath.Join(generationPath, hash[:2])
	objectPath := filepath.Join(directoryPath, hash[2:])

	if err := os.MkdirAll(directoryPath, 0o755); err != nil {
		return fmt.Errorf("create encrypted object directory %q: %w", directoryPath, err)
	}

	temp, err := os.CreateTemp(directoryPath, "."+hash[2:]+".rekey-*")
	if err != nil {
		return fmt.Errorf("create temporary encrypted object in %q: %w", directoryPath, err)
	}

	tempPath := temp.Name()
	committed := false

	defer func() {
		if committed {
			return
		}

		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()

	encoder, err := o.encryptionHandler.Encode(temp, key)
	if err != nil {
		return fmt.Errorf("create encryption encoder for object %q: %w", hash, err)
	}

	hasher := sha256.New()
	input := io.TeeReader(source, hasher)

	if _, err := io.Copy(encoder.Writer, input); err != nil {
		_ = encoder.Closer()
		return fmt.Errorf("encrypt object %q: %w", hash, err)
	}

	if err := encoder.Closer(); err != nil {
		return fmt.Errorf("close encryption encoder for object %q: %w", hash, err)
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if actualHash != hash {
		return fmt.Errorf("rekey object hash mismatch: expected %q, got %q", hash, actualHash)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync temporary encrypted object %q: %w", tempPath, err)
	}

	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary encrypted object %q: %w", tempPath, err)
	}

	if err := os.Rename(tempPath, objectPath); err != nil {
		return fmt.Errorf("commit rekeyed object %q: %w", objectPath, err)
	}

	committed = true

	return nil
}

func (o ObjectRepository) DeleteEncryptedGeneration(ctx context.Context, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	generationPath := object.EncryptedGenerationPath(o.repositoryPath, generation, false)

	if err := os.RemoveAll(generationPath); err != nil {
		return fmt.Errorf("delete encrypted generation %d from %q: %w", generation, generationPath, err)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	return nil
}

func (o ObjectRepository) AbortRekey(ctx context.Context, generation uint64) error {
	if err := o.DeleteEncryptedGeneration(ctx, generation); err != nil {
		return fmt.Errorf("abort rekey for generation %d: %w", generation, err)
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

	reader, err := os.Open(dest.ObjectPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, snapshot.NewNotFoundError(hash)
		}

		return nil, fmt.Errorf("open object %q: %w", dest.ObjectPath, err)
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

func (o ObjectRepository) AlreadyExists(ctx context.Context, hash string, symmetricKey []byte, generation uint64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, false)
	destinationPath := dest.DirectoryPath
	_, err := os.Stat(destinationPath)
	switch {
	case err == nil:
		return true, nil
	case !errors.Is(err, os.ErrNotExist):
		return false, fmt.Errorf("check destination file %q: %w", destinationPath, err)
	}
	return false, nil
}

func (o ObjectRepository) Save(ctx context.Context, filePath, hash string, comp *repository.Compression, symmetricKey []byte, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, false)
	destinationPath := dest.DirectoryPath
	objectPath := dest.ObjectPath
	if err := os.MkdirAll(destinationPath, 0o755); err != nil {
		return fmt.Errorf(
			"create object directory %q: %w",
			filepath.Dir(destinationPath),
			err,
		)
	}

	source, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", filePath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	destination, err := os.OpenFile(
		objectPath,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0o644,
	)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}

		return fmt.Errorf("create object file %q: %w", objectPath, err)
	}

	writeCompleted := false
	defer func() {
		_ = destination.Close()

		if !writeCompleted {
			_ = os.Remove(destinationPath)
		}
	}()

	encoder, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return err
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
	writer, err := encoder.Encode(output, comp.Level())
	if err != nil {
		if closeEncryption != nil {
			_ = closeEncryption()
		}
		return err
	}
	if _, err := io.Copy(writer.Writer, source); err != nil {
		return fmt.Errorf("copy object: %w", err)
	}

	if writer.Closer != nil {
		if err := writer.Closer(); err != nil {
			return fmt.Errorf(
				"close destination file %q: %w",
				destinationPath,
				err,
			)
		}
	}
	if closeEncryption != nil {
		_ = closeEncryption()
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

	hashes, err := objectHashes(obj)
	if err != nil {
		return err
	}

	destinationPath := obj.Path()

	if err := os.MkdirAll(
		filepath.Dir(destinationPath),
		0o755,
	); err != nil {
		return fmt.Errorf(
			"create destination directory for %q: %w",
			destinationPath,
			err,
		)
	}

	destination, err := os.OpenFile(
		destinationPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		0o644,
	)
	if err != nil {
		return fmt.Errorf(
			"create restored file %q: %w",
			destinationPath,
			err,
		)
	}

	restoreCompleted := false

	defer func() {
		if !restoreCompleted {
			_ = destination.Close()
			_ = os.Remove(destinationPath)
		}
	}()

	var restoredSize int64

	for index, hash := range hashes {
		if err := ctx.Err(); err != nil {
			return err
		}

		written, err := o.restoreObjectPart(
			ctx,
			comp,
			hash,
			destination,
			symmetricKey,
			generation,
		)
		if err != nil {
			return fmt.Errorf("restore part %d/%d of %q: %w", index+1, len(hashes), destinationPath, err)
		}

		restoredSize += written
	}

	if restoredSize != obj.Size() {
		return fmt.Errorf("restored size mismatch for %q: expected %d, restored %d", destinationPath, obj.Size(), restoredSize)
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf("close restored file %q: %w", destinationPath, err)
	}

	restoreCompleted = true

	return nil
}

func (o ObjectRepository) restoreObjectPart(ctx context.Context, comp *repository.Compression, hash string, destination io.Writer, symmetricKey []byte, generation uint64) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, false)
	sourcePath := dest.ObjectPath

	source, err := os.Open(sourcePath)
	if err != nil {
		return 0, fmt.Errorf("open object %q: %w", sourcePath, err)
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

	compressionHandler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return 0, err
	}

	decoded, err := compressionHandler.Decode(reader)
	if err != nil {
		return 0, fmt.Errorf("decode object %q using %q: %w", sourcePath, comp.CompType(), err)
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
		return 0, fmt.Errorf("read decoded object %q: %w", sourcePath, err)
	}

	return written, nil
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

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, key, false)
	destinationPath := dest.DirectoryPath
	objectPath := dest.ObjectPath
	if err := os.MkdirAll(
		destinationPath,
		0o755,
	); err != nil {
		return fmt.Errorf("create object directory %q: %w", destinationPath, err)
	}

	destination, err := os.Create(objectPath)
	if err != nil {
		return fmt.Errorf("create object %q: %w", objectPath, err)
	}

	success := false

	defer func() {
		_ = destination.Close()

		if !success {
			_ = os.Remove(destinationPath)
		}
	}()

	handler, err := o.handlersFactory.GetHandler(
		comp.CompType(),
	)
	if err != nil {
		return err
	}

	output := io.Writer(destination)
	var closeEncryption func() error

	if len(key) > 0 {
		encrypted, err := o.encryptionHandler.Encode(destination, key)
		if err != nil {
			return fmt.Errorf("create encryption writer: %w", err)
		}

		output = encrypted.Writer
		closeEncryption = encrypted.Closer
	}

	encoder, err := handler.Encode(
		output,
		comp.Level(),
	)
	if err != nil {
		if closeEncryption != nil {
			_ = closeEncryption()
		}
		return err
	}

	section := io.NewSectionReader(
		source,
		offset,
		size,
	)

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
		if closeEncryption != nil {
			_ = closeEncryption()
		}

		return fmt.Errorf("write chunk from %q at offset %d: %w", filePath, offset, copyErr)
	}

	if encoder.Closer != nil {
		if err := encoder.Closer(); err != nil {
			return fmt.Errorf("finish chunk compression for %q: %w", filePath, err)
		}
	}

	if closeEncryption != nil {
		if err := closeEncryption(); err != nil {
			return fmt.Errorf("finish chunk encryption for %q: %w", filePath, err)
		}
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf("close object %q: %w", destinationPath, err)
	}

	success = true

	return nil
}

func NewObjectRepository(repositoryPath string) (*ObjectRepository, error) {
	absPath, err := absolutePath(repositoryPath)
	if err != nil {
		return nil, err
	}
	handlersFactory := compression.NewHandlersFactory()
	repo := ObjectRepository{
		repositoryPath:    absPath,
		handlersFactory:   handlersFactory,
		encryptionHandler: encrypt.NewAESGCMHandler(),
	}
	return &repo, nil
}

func objectHashes(obj *snapshot.File) ([]string, error) {
	chunks := obj.Chunks()

	if len(chunks) > 0 {
		hashes := make([]string, 0, len(chunks))

		for index, chunk := range chunks {
			hash := chunk.Hash()

			if len(hash) < 3 {
				return nil, fmt.Errorf("invalid hash for chunk %d: %q", index+1, hash)
			}

			hashes = append(hashes, hash)
		}

		return hashes, nil
	}

	hash := obj.Hash()

	if len(hash) < 3 {
		return nil, fmt.Errorf("invalid object hash %q", hash)
	}

	return []string{hash}, nil
}
