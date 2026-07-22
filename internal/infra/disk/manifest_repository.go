package disk

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bruli-lab/stowmark.git/internal/domain/repository"
	"github.com/bruli-lab/stowmark.git/internal/domain/snapshot"
)

type file struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

type manifest struct {
	ID        string    `json:"id"`
	Files     []file    `json:"files"`
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source"`
}

type ManifestRepository struct {
	repositoryPath string
}

func (r ManifestRepository) Save(ctx context.Context, m *snapshot.Manifest) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	files := make([]file, len(m.Files()))
	for i, f := range m.Files() {
		files[i] = file{
			Path: f.Path(),
			Hash: f.Hash(),
			Size: f.Size(),
		}
	}
	man := manifest{
		ID:        m.Id(),
		Files:     files,
		CreatedAt: m.CreatedAt(),
		Source:    m.Source(),
	}
	data, err := json.Marshal(man)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}
	manFile := fmt.Sprintf("%s.json", m.Id())
	return os.WriteFile(filepath.Join(fmt.Sprintf("%s/%s", r.repositoryPath, repository.SnapshotsFolder), manFile), data, 0o644)
}

func NewManifestRepository(repositoryPath string) *ManifestRepository {
	return &ManifestRepository{repositoryPath: repositoryPath}
}
