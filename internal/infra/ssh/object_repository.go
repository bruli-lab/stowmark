package ssh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/pkg/sftp"
)

type ObjectRepository struct {
	client          *sftp.Client
	handlersFactory *compression.HandlersFactory
	repositoryPath  string
}

func (o ObjectRepository) RestoreObject(ctx context.Context, comp *repository.Compression, obj *snapshot.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if comp == nil {
		return errors.New("compression configuration is required")
	}

	if obj == nil {
		return errors.New("snapshot object is required")
	}

	hash := obj.Hash()
	if len(hash) < 3 {
		return fmt.Errorf("invalid object hash %q", hash)
	}

	sourcePath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	source, err := o.client.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open remote object %q: %w", sourcePath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return fmt.Errorf(
			"get compression handler %q: %w",
			comp.CompType(),
			err,
		)
	}

	decoded, err := handler.Decode(source)
	if err != nil {
		return fmt.Errorf(
			"decode remote object %q using %q: %w",
			sourcePath,
			comp.CompType(),
			err,
		)
	}
	defer decoded.Closer()

	destinationPath := obj.Path()

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return fmt.Errorf(
			"create local destination directory for %q: %w",
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
			"create local restored file %q: %w",
			destinationPath,
			err,
		)
	}

	restoreCompleted := false

	defer func() {
		if restoreCompleted {
			return
		}

		_ = destination.Close()
		_ = os.Remove(destinationPath)
	}()

	if _, err := io.Copy(
		destination,
		contextReader{
			ctx:    ctx,
			reader: decoded.Reader,
		},
	); err != nil {
		return fmt.Errorf(
			"restore remote object %q to local file %q: %w",
			sourcePath,
			destinationPath,
			err,
		)
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf(
			"close local restored file %q: %w",
			destinationPath,
			err,
		)
	}

	restoreCompleted = true

	return nil
}

func (o ObjectRepository) ReadObject(ctx context.Context, originalPath, hash string) (*snapshot.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(hash) < 3 {
		return nil, fmt.Errorf("invalid object hash %q", hash)
	}

	objectPath := filepath.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	objectFile, err := o.client.Open(objectPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, snapshot.NewNotFoundError(originalPath)
		}

		return nil, fmt.Errorf(
			"open object %q: %w",
			objectPath,
			err,
		)
	}
	defer func() {
		_ = objectFile.Close()
	}()

	hasher := sha256.New()

	storedSize, err := io.Copy(
		hasher,
		contextReader{
			ctx:    ctx,
			reader: objectFile,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"read object %q: %w",
			objectPath,
			err,
		)
	}

	calculatedHash := hex.EncodeToString(hasher.Sum(nil))

	result := snapshot.File{}
	result.Hydrate(
		originalPath,
		calculatedHash,
		storedSize,
	)

	return &result, nil
}

func (o ObjectRepository) AlreadyExists(ctx context.Context, obj *snapshot.File) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if obj == nil {
		return false, errors.New("snapshot object is required")
	}

	hash := obj.Hash()
	if len(hash) < 3 {
		return false, fmt.Errorf("invalid object hash %q", hash)
	}

	destinationPath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

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

func (o ObjectRepository) Save(ctx context.Context, obj *snapshot.File, comp *repository.Compression) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if obj == nil {
		return errors.New("snapshot object is required")
	}

	if comp == nil {
		return errors.New("compression configuration is required")
	}

	hash := obj.Hash()
	if len(hash) < 3 {
		return fmt.Errorf("invalid object hash %q", hash)
	}

	destinationPath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	destinationDir := path.Dir(destinationPath)

	if err := o.client.MkdirAll(destinationDir); err != nil {
		return fmt.Errorf("create remote object directory %q: %w", destinationDir, err)
	}

	source, err := os.Open(obj.Path())
	if err != nil {
		return fmt.Errorf("open local source file %q: %w", obj.Path(), err)
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

	encoded, err := handler.Encode(destination, comp.Level())
	if err != nil {
		return fmt.Errorf("encode remote object %q using %q: %w", destinationPath, comp.CompType(), err)
	}

	if _, err := io.Copy(encoded.Writer, contextReader{ctx: ctx, reader: source}); err != nil {
		return fmt.Errorf("write local file %q to remote object %q: %w", obj.Path(), destinationPath, err)
	}

	if encoded.Closer != nil {
		if err := encoded.Closer(); err != nil {
			return fmt.Errorf("close %q encoder for remote object %q: %w", comp.CompType(), destinationPath, err)
		}
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf("close remote object file %q: %w", destinationPath, err)
	}

	writeCompleted = true

	return nil
}

func NewObjectRepository(repositoryPath string, client *sftp.Client) *ObjectRepository {
	return &ObjectRepository{
		repositoryPath:  repositoryPath,
		handlersFactory: compression.NewHandlersFactory(),
		client:          client,
	}
}
