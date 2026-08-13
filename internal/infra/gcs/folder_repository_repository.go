package gcs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"time"

	"cloud.google.com/go/storage"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/google/uuid"
)

type FolderRepositoryRepository struct {
	client *storage.Client
	bucket string
}

func (f FolderRepositoryRepository) Exists(ctx context.Context, repositoryPath string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	objectPath := path.Join(repositoryPath, model.ConfigFile)

	_, err := f.client.
		Bucket(f.bucket).
		Object(objectPath).
		Attrs(ctx)

	if errors.Is(err, storage.ErrObjectNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("get attributes for config %q in bucket %q: %w", objectPath, f.bucket, err)
	}

	return true, nil
}

func (f FolderRepositoryRepository) CreateFolder(ctx context.Context, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return nil
}

func (f FolderRepositoryRepository) CreateConfig(ctx context.Context, repositoryPath string, c *repository.Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if c == nil {
		return errors.New("repository config is required")
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
		return fmt.Errorf("marshal config: %w", err)
	}

	data = append(data, '\n')

	configPath := path.Join(repositoryPath, model.ConfigFile)

	writeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	writer := f.client.
		Bucket(f.bucket).
		Object(configPath).
		NewWriter(writeCtx)

	writer.ContentType = "application/json"

	n, err := writer.Write(data)
	if err != nil {
		cancel()

		return fmt.Errorf("write config %q in bucket %q: %w", configPath, f.bucket, err)
	}

	if n != len(data) {
		cancel()

		return fmt.Errorf("write config %q in bucket %q: %w", configPath, f.bucket, io.ErrShortWrite)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("close config %q in bucket %q: %w", configPath, f.bucket, err)
	}
	return nil
}

func (f FolderRepositoryRepository) GetConfig(ctx context.Context, repositoryPath string) (*repository.Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	configPath := path.Join(repositoryPath, model.ConfigFile)

	reader, err := f.client.
		Bucket(f.bucket).
		Object(configPath).
		NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, repository.NewNotFoundError("config file not found")
		}

		return nil, fmt.Errorf("open config %q in bucket %q: %w", configPath, f.bucket, err)
	}

	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()

	if readErr != nil {
		return nil, fmt.Errorf("read config %q in bucket %q: %w", configPath, f.bucket, readErr)
	}

	if closeErr != nil {
		return nil, fmt.Errorf("close config %q in bucket %q: %w", configPath, f.bucket, closeErr)
	}

	var conf model.Config
	if err := json.Unmarshal(data, &conf); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	id, err := uuid.Parse(conf.ID)
	if err != nil {
		return nil, fmt.Errorf("parse config id: %w", err)
	}

	compType, err := repository.ParseCompressionType(
		conf.Compression.Type,
	)
	if err != nil {
		return nil, fmt.Errorf("parse compression type: %w", err)
	}

	comp, err := repository.NewCompression(
		*compType,
		conf.Compression.Level,
	)
	if err != nil {
		return nil, fmt.Errorf("create compression: %w", err)
	}

	return repository.NewConfig(id, comp), nil
}

func NewFolderRepositoryRepository(client *storage.Client, bucket string) *FolderRepositoryRepository {
	return &FolderRepositoryRepository{client: client, bucket: bucket}
}
