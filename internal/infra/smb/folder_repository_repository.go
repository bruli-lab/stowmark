package smb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/codec"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/cloudsoda/go-smb2"
	"github.com/google/uuid"
)

type FolderRepositoryRepository struct {
	share *smb2.Share
}

func (f FolderRepositoryRepository) Exists(ctx context.Context, folderPath string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	info, err := f.share.WithContext(ctx).Stat(folderPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat SMB directory %q: %w", folderPath, err)
	}

	return info.IsDir(), nil
}

func (f FolderRepositoryRepository) CreateFolder(ctx context.Context, folderPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := f.share.WithContext(ctx).MkdirAll(folderPath, 0o755); err != nil {
		return fmt.Errorf("create SMB directory %q: %w", folderPath, err)
	}

	return nil
}

func (f FolderRepositoryRepository) CreateConfig(ctx context.Context, folderPath string, c *repository.Config) error {
	data, err := codec.MarshalConfig(ctx, c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	configPath := path.Join(folderPath, model.ConfigFile)
	share := f.share.WithContext(ctx)

	file, err := share.OpenFile(
		configPath,
		os.O_WRONLY|os.O_CREATE,
		0o644,
	)
	if err != nil {
		return fmt.Errorf("open config %q: %w", configPath, err)
	}

	if _, err := file.WriteAt(data, 0); err != nil {
		_ = file.Close()

		return fmt.Errorf("write config %q: %w", configPath, err)
	}

	if err := file.Truncate(int64(len(data))); err != nil {
		_ = file.Close()

		return fmt.Errorf("truncate config %q: %w", configPath, err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close config %q: %w", configPath, err)
	}

	return nil
}

func (f FolderRepositoryRepository) GetConfig(ctx context.Context, folderPath string) (*repository.Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	configPath := path.Join(folderPath, model.ConfigFile)

	data, err := f.share.WithContext(ctx).ReadFile(configPath)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return nil, repository.NewNotFoundError("config file not found")
		default:
			return nil, fmt.Errorf("read SMB config %q: %w", configPath, err)
		}
	}

	var conf model.Config
	if err := json.Unmarshal(data, &conf); err != nil {
		return nil, fmt.Errorf("unmarshal config %q: %w", configPath, err)
	}

	id, err := uuid.Parse(conf.ID)
	if err != nil {
		return nil, fmt.Errorf("parse config ID: %w", err)
	}

	compType, err := repository.ParseCompressionType(conf.Compression.Type)
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

	return repository.NewConfig(id, conf.FormatVersion, comp), nil
}

func NewFolderRepositoryRepository(share *smb2.Share) *FolderRepositoryRepository {
	return &FolderRepositoryRepository{share: share}
}
