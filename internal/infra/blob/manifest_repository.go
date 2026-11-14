package blob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/model"
)

type ManifestRepository struct {
	store Store
}

func (m ManifestRepository) Save(ctx context.Context, manifest *snapshot.Manifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key, err := manifestKey(manifest.ID())
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(model.NewManifest(manifest), "", " ")
	if err != nil {
		return fmt.Errorf("marshal manifest %q: %w", manifest.ID(), err)
	}
	data = append(data, '\n')

	return m.store.Put(ctx, key, func(destination io.Writer) error {
		_, err := destination.Write(data)
		return err
	})
}

func (m ManifestRepository) Get(ctx context.Context, snapshotID string) (*snapshot.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key, err := manifestKey(snapshotID)
	if err != nil {
		return nil, err
	}

	return m.read(ctx, key)
}

func (m ManifestRepository) List(ctx context.Context) ([]snapshot.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	keys, err := m.store.List(ctx, repository.SnapshotsFolder)
	if err != nil {
		return nil, fmt.Errorf("list manifests: %w", err)
	}

	manifests := make([]snapshot.Manifest, 0, len(keys))
	for _, key := range keys {
		if !isManifestKey(key) {
			continue
		}

		manifest, err := m.read(ctx, key)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, *manifest)
	}

	slices.SortFunc(manifests, func(a, b snapshot.Manifest) int {
		return b.CreatedAt().Compare(a.CreatedAt())
	})

	return manifests, nil
}

func (m ManifestRepository) read(ctx context.Context, key string) (*snapshot.Manifest, error) {
	reader, err := m.store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, repository.NewNotFoundError("manifest not found")
		}
		return nil, fmt.Errorf("open manifest %q: %w", key, err)
	}
	defer func() {
		_ = reader.Close()
	}()

	data, err := io.ReadAll(model.ContextReader{Ctx: ctx, Reader: reader})
	if err != nil {
		return nil, fmt.Errorf("read manifest %q: %w", key, err)
	}

	return model.BuildManifestDomain(data, key)
}

func NewManifestRepository(store Store) *ManifestRepository {
	return &ManifestRepository{store: store}
}
