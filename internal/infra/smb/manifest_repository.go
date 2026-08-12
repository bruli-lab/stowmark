package smb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/cloudsoda/go-smb2"
)

type ManifestRepository struct {
	repositoryPath string
	share          *smb2.Share
}

func (r ManifestRepository) Save(ctx context.Context, m *snapshot.Manifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	files := make([]model.File, len(m.Files()))
	for i, f := range m.Files() {
		files[i] = model.File{
			Path: f.Path(),
			Hash: f.Hash(),
			Size: f.Size(),
		}
	}

	man := model.Manifest{
		ID:        m.Id(),
		Files:     files,
		CreatedAt: m.CreatedAt().In(time.Local),
		Source:    m.Source(),
		Compression: model.Compression{
			Type:  m.Compression().CompType().String(),
			Level: m.Compression().Level(),
		},
	}

	data, err := json.MarshalIndent(man, "", " ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	data = append(data, '\n')

	manifestPath := path.Join(
		r.repositoryPath,
		repository.SnapshotsFolder,
		fmt.Sprintf("%s.json", m.Id()),
	)

	file, err := r.share.OpenFile(
		manifestPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		0o644,
	)
	if err != nil {
		return fmt.Errorf("open manifest %q: %w", manifestPath, err)
	}

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write manifest %q: %w", manifestPath, err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close manifest %q: %w", manifestPath, err)
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

	entries, err := r.share.ReadDir(snapshotsPath)
	if err != nil {
		return nil, fmt.Errorf("read snapshots folder %q: %w", snapshotsPath, err)
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

		data, err := r.share.ReadFile(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("read manifest %q: %w", manifestPath, err)
		}

		manifest, err := r.buildManifest(data, manifestPath)
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

func (r ManifestRepository) Get(ctx context.Context, snapshotID string) (*snapshot.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	filePath := path.Join(
		r.repositoryPath,
		repository.SnapshotsFolder,
		fmt.Sprintf("%s.json", snapshotID),
	)

	data, err := r.share.ReadFile(filePath)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return nil, repository.NewNotFoundError("manifest not found")
		default:
			return nil, fmt.Errorf(
				"read manifest %q: %w",
				filePath,
				err,
			)
		}
	}

	return r.buildManifest(data, filePath)
}

func (r ManifestRepository) buildManifest(data []byte, manifestPath string) (*snapshot.Manifest, error) {
	var mo model.Manifest
	if err := json.Unmarshal(data, &mo); err != nil {
		return nil, fmt.Errorf(
			"failed to unmarshal manifest %q: %w",
			manifestPath,
			err,
		)
	}
	snapshotFiles := make(
		[]snapshot.File,
		len(mo.Files),
	)
	for i, modelFile := range mo.Files {
		file := snapshot.File{}
		file.Hydrate(modelFile.Path, modelFile.Hash, modelFile.Size)
		snapshotFiles[i] = file
	}
	compType, err := repository.ParseCompressionType(mo.Compression.Type)
	if err != nil {
		return nil, err
	}
	comp, err := repository.NewCompression(*compType, mo.Compression.Level)
	if err != nil {
		return nil, err
	}
	man := snapshot.NewManifest(
		mo.ID,
		snapshotFiles,
		mo.CreatedAt,
		mo.Source,
		comp,
	)
	return man, nil
}

func NewManifestRepository(repositoryPath string, share *smb2.Share) *ManifestRepository {
	return &ManifestRepository{repositoryPath: repositoryPath, share: share}
}
