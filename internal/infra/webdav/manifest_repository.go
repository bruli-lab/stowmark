package webdav

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/studio-b12/gowebdav"
)

type ManifestRepository struct {
	client         *gowebdav.Client
	repositoryPath string
}

func (r ManifestRepository) Save(ctx context.Context, m *snapshot.Manifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	files := make([]model.File, len(m.Files()))
	for i, file := range m.Files() {
		files[i] = model.File{
			Path: file.Path(),
			Hash: file.Hash(),
			Size: file.Size(),
		}
	}

	manifest := model.Manifest{
		ID:        m.Id(),
		Files:     files,
		CreatedAt: m.CreatedAt().In(time.Local),
		Source:    m.Source(),
		Compression: model.Compression{
			Type:  m.Compression().CompType().String(),
			Level: m.Compression().Level(),
		},
	}

	data, err := json.MarshalIndent(manifest, "", " ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	data = append(data, '\n')

	manifestPath := path.Join(
		r.repositoryPath,
		repository.SnapshotsFolder,
		fmt.Sprintf("%s.json", m.Id()),
	)

	if err := r.client.Write(manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("write WebDAV manifest %q: %w", manifestPath, err)
	}

	return nil
}

func (r ManifestRepository) List(ctx context.Context) ([]snapshot.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	snapshotsPath := path.Join(
		r.repositoryPath,
		repository.SnapshotsFolder,
	)

	entries, err := r.client.ReadDir(snapshotsPath)
	if err != nil {
		return nil, fmt.Errorf("read WebDAV snapshots folder %q: %w", snapshotsPath, err)
	}

	manifests := make([]snapshot.Manifest, 0, len(entries))

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if entry.IsDir() {
			continue
		}

		manifestPath := path.Join(
			snapshotsPath,
			entry.Name(),
		)

		data, err := r.client.Read(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("read WebDAV manifest %q: %w", manifestPath, err)
		}

		manifest, err := model.BuildManifestDomain(data, manifestPath)
		if err != nil {
			return nil, fmt.Errorf("build manifest from %q: %w", manifestPath, err)
		}

		manifests = append(manifests, *manifest)
	}

	slices.SortFunc(
		manifests,
		func(a, b snapshot.Manifest) int {
			return b.CreatedAt().Compare(a.CreatedAt())
		},
	)

	return manifests, nil
}

func (r ManifestRepository) Get(ctx context.Context, snapshotID string) (*snapshot.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	manifestPath := path.Join(
		r.repositoryPath,
		repository.SnapshotsFolder,
		fmt.Sprintf("%s.json", snapshotID),
	)

	data, err := r.client.Read(manifestPath)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, repository.NewNotFoundError("manifest not found")
		}

		return nil, fmt.Errorf("read WebDAV manifest %q: %w", manifestPath, err)
	}

	manifest, err := model.BuildManifestDomain(data, manifestPath)
	if err != nil {
		return nil, fmt.Errorf("build manifest from %q: %w", manifestPath, err)
	}

	return manifest, nil
}

func NewManifestRepository(client *gowebdav.Client, repositoryPath string) *ManifestRepository {
	return &ManifestRepository{client: client, repositoryPath: repositoryPath}
}
