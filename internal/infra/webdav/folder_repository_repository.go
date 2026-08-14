package webdav

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/google/uuid"
	"github.com/studio-b12/gowebdav"
)

type FolderRepositoryRepository struct {
	client *gowebdav.Client
}

func (f FolderRepositoryRepository) Exists(ctx context.Context, repositoryPath string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	info, err := f.client.Stat(repositoryPath)
	if gowebdav.IsErrNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat WebDAV path %q: %w", repositoryPath, err)
	}

	return info.IsDir(), nil
}

func (f FolderRepositoryRepository) CreateFolder(ctx context.Context, repositoryPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	remotePath := webDAVPath(repositoryPath)
	if err := f.client.MkdirAll(remotePath, 0o755); err != nil {
		return fmt.Errorf("create WebDAV folder %q: %w", remotePath, err)
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
		return fmt.Errorf("marshal config: %w", err)
	}

	data = append(data, '\n')

	configPath := path.Join(repositoryPath, model.ConfigFile)
	if err := f.client.Write(configPath, data, 0o644); err != nil {
		return fmt.Errorf("write WebDAV config %q: %w", configPath, err)
	}

	return nil
}

func (f FolderRepositoryRepository) GetConfig(ctx context.Context, repositoryPath string) (*repository.Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	configPath := path.Join(repositoryPath, model.ConfigFile)

	data, err := f.client.Read(configPath)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, repository.NewNotFoundError("config file not found")
		}

		return nil, fmt.Errorf(
			"read WebDAV config %q: %w",
			configPath,
			err,
		)
	}

	var conf model.Config
	if err := json.Unmarshal(data, &conf); err != nil {
		return nil, fmt.Errorf(
			"unmarshal WebDAV config %q: %w",
			configPath,
			err,
		)
	}

	id, err := uuid.Parse(conf.ID)
	if err != nil {
		return nil, fmt.Errorf("parse config id: %w", err)
	}

	compType, err := repository.ParseCompressionType(conf.Compression.Type)
	if err != nil {
		return nil, fmt.Errorf("parse compression type: %w", err)
	}

	compression, err := repository.NewCompression(
		*compType,
		conf.Compression.Level,
	)
	if err != nil {
		return nil, fmt.Errorf("create compression: %w", err)
	}

	return repository.NewConfig(id, conf.FormatVersion, compression), nil
}

func NewFolderRepositoryRepository(client *gowebdav.Client) *FolderRepositoryRepository {
	return &FolderRepositoryRepository{client: client}
}

func webDAVPath(value string) string {
	return strings.TrimPrefix(path.Clean("/"+value), "/")
}
