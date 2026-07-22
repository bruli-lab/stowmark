package disk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bruli-lab/stowmark.git/internal/domain/repository"
	"github.com/google/uuid"
)

const configFile = "config.json"

type compression struct {
	Type  string `json:"type"`
	Level *int   `json:"level,omitempty"`
}
type config struct {
	ID            string      `json:"id"`
	FormatVersion int         `json:"format_version"`
	CreatedAt     string      `json:"created_at"`
	Compression   compression `json:"compression"`
}

type FolderRepositoryRepository struct{}

func (f FolderRepositoryRepository) GetConfig(ctx context.Context, path string) (*repository.Config, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	absolutePath, err := filepath.Abs(path)
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
	var conf config
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
	return repository.NewConfig(id, repository.NewCompression(*compType, conf.Compression.Level)), nil
}

func (f FolderRepositoryRepository) Exists(ctx context.Context, name string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}
	info, err := os.Stat(name)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func (f FolderRepositoryRepository) CreateFolder(ctx context.Context, name string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return os.MkdirAll(name, 0o755)
}

func (f FolderRepositoryRepository) CreateConfig(ctx context.Context, path string, c *repository.Config) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	co := config{
		ID:            c.Id().String(),
		FormatVersion: c.FormatVersion(),
		CreatedAt:     c.CreatedAt().Format(time.RFC3339),
		Compression: compression{
			Type:  c.Compression().CompType().String(),
			Level: c.Compression().Level(),
		},
	}
	data, err := json.Marshal(co)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	return os.WriteFile(filepath.Join(path, configFile), data, 0o644)
}

func NewFolderRepositoryRepository() *FolderRepositoryRepository {
	return &FolderRepositoryRepository{}
}
