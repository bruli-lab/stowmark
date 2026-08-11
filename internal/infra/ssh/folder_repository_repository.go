package ssh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/google/uuid"
	"github.com/pkg/sftp"
)

type FolderRepositoryRepository struct {
	client *sftp.Client
}

func NewFolderRepositoryRepository(client *sftp.Client) FolderRepositoryRepository {
	return FolderRepositoryRepository{
		client: client,
	}
}

func (f FolderRepositoryRepository) Exists(ctx context.Context, repositoryPath string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	_, err := f.client.Stat(repositoryPath)
	switch {
	case err == nil:
		return true, nil

	case errors.Is(err, fs.ErrNotExist):
		return false, nil

	default:
		return false, fmt.Errorf(
			"stat remote repository %q: %w",
			repositoryPath,
			err,
		)
	}
}

func (f FolderRepositoryRepository) CreateFolder(ctx context.Context, folderPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := f.client.MkdirAll(folderPath); err != nil {
		return fmt.Errorf(
			"create remote folder %q: %w",
			folderPath,
			err,
		)
	}

	return nil
}

func (f FolderRepositoryRepository) CreateConfig(ctx context.Context, repositoryPath string, c *repository.Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	co := model.Config{
		ID:            c.Id().String(),
		FormatVersion: c.FormatVersion(),
		CreatedAt:     c.CreatedAt().In(time.Local).Format(time.RFC3339),
		Compression: model.Compression{
			Type:  c.Compression().CompType().String(),
			Level: c.Compression().Level(),
		},
	}

	data, err := json.MarshalIndent(co, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal repository config: %w", err)
	}

	data = append(data, '\n')

	configPath := path.Join(repositoryPath, model.ConfigFile)
	temporaryPath := configPath + ".tmp"

	if err := f.writeFile(ctx, temporaryPath, data); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		_ = f.client.Remove(temporaryPath)
		return err
	}

	if err := f.client.PosixRename(temporaryPath, configPath); err != nil {
		_ = f.client.Remove(temporaryPath)

		return fmt.Errorf("replace remote config %q: %w", configPath, err)
	}

	return nil
}

func (f FolderRepositoryRepository) GetConfig(ctx context.Context, repositoryPath string) (*repository.Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	configPath := path.Join(repositoryPath, model.ConfigFile)

	file, err := f.client.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf(
			"open remote config %q: %w",
			configPath,
			err,
		)
	}
	defer func() {
		_ = file.Close()
	}()

	var conf model.Config

	if err := decodeJSON(ctx, file, &conf); err != nil {
		return nil, fmt.Errorf(
			"decode remote config %q: %w",
			configPath,
			err,
		)
	}

	id, err := uuid.Parse(conf.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config id: %w", err)
	}
	compType, err := repository.ParseCompressionType(conf.Compression.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to parse compression type: %w", err)
	}
	comp, err := repository.NewCompression(*compType, conf.Compression.Level)
	if err != nil {
		return nil, fmt.Errorf("failed to create compression: %w", err)
	}
	return repository.NewConfig(id, comp), nil
}

func (f FolderRepositoryRepository) writeFile(ctx context.Context, filePath string, data []byte) error {
	file, err := f.client.Create(filePath)
	if err != nil {
		return fmt.Errorf(
			"create remote file %q: %w",
			filePath,
			err,
		)
	}

	_, writeErr := io.Copy(
		file,
		contextReader{
			ctx:    ctx,
			reader: bytes.NewReader(data),
		},
	)

	closeErr := file.Close()

	if writeErr != nil {
		_ = f.client.Remove(filePath)

		return fmt.Errorf(
			"write remote file %q: %w",
			filePath,
			writeErr,
		)
	}

	if closeErr != nil {
		return fmt.Errorf(
			"close remote file %q: %w",
			filePath,
			closeErr,
		)
	}

	return nil
}

func decodeJSON(ctx context.Context, reader io.Reader, value any) error {
	return json.NewDecoder(
		contextReader{
			ctx:    ctx,
			reader: reader,
		},
	).Decode(value)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}

	return r.reader.Read(data)
}
