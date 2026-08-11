package disk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/google/uuid"
)

const configFile = "config.json"



type FolderRepositoryRepository struct{}

func (f FolderRepositoryRepository) GetConfig(ctx context.Context, path string) (*repository.Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	absolutePath, err := absolutePath(path)
	if err != nil {
		return nil, fmt.Errorf("resolve absolute path: %w", err)
	}
	configPath := filepath.Join(absolutePath, configFile)
	data, err := os.ReadFile(configPath)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return nil, repository.NewNotFoundError("config file not found")
		default:
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}
	var conf model.Config
	if err := json.Unmarshal(data, &conf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
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

func (f FolderRepositoryRepository) Exists(ctx context.Context, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func (f FolderRepositoryRepository) CreateFolder(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.MkdirAll(path, 0o755)
}

func (f FolderRepositoryRepository) CreateConfig(ctx context.Context, path string, c *repository.Config) error {
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

	configPath := filepath.Join(path, configFile)
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("write config %q: %w", configPath, err)
	}

	return nil
}

func NewFolderRepositoryRepository() *FolderRepositoryRepository {
	return &FolderRepositoryRepository{}
}
