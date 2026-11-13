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
	if err := writeBytesAtomically(f.share.WithContext(ctx), configPath, data); err != nil {
		return fmt.Errorf("write config %q: %w", configPath, err)
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

	return model.BuildConfigDomain(conf)
}

func NewFolderRepositoryRepository(share *smb2.Share) *FolderRepositoryRepository {
	return &FolderRepositoryRepository{share: share}
}
