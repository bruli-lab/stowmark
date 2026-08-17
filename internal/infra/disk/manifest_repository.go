package disk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/model"
)

type ManifestRepository struct {
	repositoryPath string
}

func (r ManifestRepository) Get(ctx context.Context, snapshotID string) (*snapshot.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshotsPath := filepath.Join(
		r.repositoryPath,
		repository.SnapshotsFolder,
	)
	filePath := filepath.Join(snapshotsPath, fmt.Sprintf("%s.json", snapshotID))
	data, err := os.ReadFile(filePath)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return nil, repository.NewNotFoundError("manifest not found")
		default:
			return nil, fmt.Errorf(
				"failed to read manifest %q: %w",
				filePath,
				err,
			)
		}
	}
	var man model.Manifest
	if err := json.Unmarshal(data, &man); err != nil {
		return nil, fmt.Errorf(
			"failed to unmarshal manifest %q: %w",
			filePath,
			err,
		)
	}
	return model.BuildManifestDomain(data, filePath)
}

func (r ManifestRepository) List(ctx context.Context) ([]snapshot.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshotsPath := filepath.Join(
		r.repositoryPath,
		repository.SnapshotsFolder,
	)
	entries, err := os.ReadDir(snapshotsPath)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to read snapshots folder: %w",
			err,
		)
	}
	manifests := make([]snapshot.Manifest, len(entries))
	for i, entry := range entries {
		if entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(
			snapshotsPath,
			entry.Name(),
		)
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to read manifest %q: %w",
				manifestPath,
				err,
			)
		}
		man, err := model.BuildManifestDomain(data, manifestPath)
		if err != nil {
			return nil, err
		}
		manifests[i] = *man
	}
	slices.SortFunc(manifests, func(a, b snapshot.Manifest) int {
		return b.CreatedAt().Compare(a.CreatedAt())
	})
	return manifests, nil
}

func (r ManifestRepository) Save(ctx context.Context, m *snapshot.Manifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	man := model.NewManifest(m)
	data, err := json.MarshalIndent(man, "", " ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}
	data = append(data, '\n')
	manFile := fmt.Sprintf("%s.json", m.Id())
	return os.WriteFile(filepath.Join(fmt.Sprintf("%s/%s", r.repositoryPath, repository.SnapshotsFolder), manFile), data, 0o644)
}

func NewManifestRepository(repositoryPath string) (*ManifestRepository, error) {
	absPath, err := absolutePath(repositoryPath)
	if err != nil {
		return nil, err
	}
	return &ManifestRepository{repositoryPath: absPath}, nil
}

func absolutePath(repositoryPath string) (string, error) {
	absolutePath, err := filepath.Abs(repositoryPath)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	return absolutePath, nil
}
