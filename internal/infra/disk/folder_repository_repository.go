package disk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bruli-lab/stonekeep.git/internal/domain/repository"
)

const configFile = "config.json"

type config struct {
	ID            string `json:"id"`
	FormatVersion int    `json:"format_version"`
	CreatedAt     string `json:"created_at"`
}

type FolderRepositoryRepository struct{}

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
