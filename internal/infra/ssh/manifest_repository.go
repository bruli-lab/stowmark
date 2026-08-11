package ssh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/pkg/sftp"
)

type ManifestRepository struct {
	repositoryPath string
	client         *sftp.Client
}

func (ma ManifestRepository) Save(ctx context.Context, m *snapshot.Manifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if m == nil {
		return errors.New("manifest is required")
	}

	files := make([]model.File, len(m.Files()))
	for i, file := range m.Files() {
		files[i] = model.File{
			Path: file.Path(),
			Hash: file.Hash(),
			Size: file.Size(),
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

	snapshotsPath := path.Join(
		ma.repositoryPath,
		repository.SnapshotsFolder,
	)

	if err := ma.client.MkdirAll(snapshotsPath); err != nil {
		return fmt.Errorf("create remote snapshots directory %q: %w", snapshotsPath, err)
	}

	manifestPath := path.Join(
		snapshotsPath,
		m.Id()+".json",
	)

	file, err := ma.client.OpenFile(
		manifestPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
	)
	if err != nil {
		return fmt.Errorf("create remote manifest %q: %w", manifestPath, err)
	}

	writeCompleted := false

	defer func() {
		_ = file.Close()

		if !writeCompleted {
			_ = ma.client.Remove(manifestPath)
		}
	}()

	if _, err := io.Copy(
		file,
		contextReader{
			ctx:    ctx,
			reader: bytes.NewReader(data),
		},
	); err != nil {
		return fmt.Errorf("write remote manifest %q: %w", manifestPath, err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close remote manifest %q: %w", manifestPath, err)
	}

	if err := ma.client.Chmod(manifestPath, 0o644); err != nil {
		return fmt.Errorf("set remote manifest permissions %q: %w", manifestPath, err)
	}

	writeCompleted = true

	return nil
}

func (ma ManifestRepository) List(ctx context.Context) ([]snapshot.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	snapshotsPath := path.Join(
		ma.repositoryPath,
		repository.SnapshotsFolder,
	)

	entries, err := ma.client.ReadDir(snapshotsPath)
	if err != nil {
		return nil, fmt.Errorf(
			"read remote snapshots folder %q: %w",
			snapshotsPath,
			err,
		)
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

		manifestFile, err := ma.client.Open(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("open remote manifest %q: %w", manifestPath, err)
		}

		data, readErr := io.ReadAll(contextReader{
			ctx:    ctx,
			reader: manifestFile,
		})
		closeErr := manifestFile.Close()

		if readErr != nil {
			return nil, fmt.Errorf("read remote manifest %q: %w", manifestPath, readErr)
		}

		if closeErr != nil {
			return nil, fmt.Errorf("close remote manifest %q: %w", manifestPath, closeErr)
		}

		manifest, err := ma.buildManifest(data, manifestPath)
		if err != nil {
			return nil, fmt.Errorf("build manifest %q: %w", manifestPath, err)
		}

		manifests = append(manifests, *manifest)
	}

	slices.SortFunc(manifests, func(a, b snapshot.Manifest) int {
		return b.CreatedAt().Compare(a.CreatedAt())
	})

	return manifests, nil
}

func (ma ManifestRepository) Get(ctx context.Context, snapshotID string) (*snapshot.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	filePath := path.Join(ma.repositoryPath, repository.SnapshotsFolder, snapshotID+".json")

	file, err := ma.client.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, repository.NewNotFoundError("manifest not found")
		}

		return nil, fmt.Errorf("open remote manifest %q: %w", filePath, err)
	}
	defer func() {
		_ = file.Close()
	}()

	data, err := io.ReadAll(
		contextReader{
			ctx:    ctx,
			reader: file,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("read remote manifest %q: %w", filePath, err)
	}

	return ma.buildManifest(data, filePath)
}

func (ma ManifestRepository) buildManifest(data []byte, manifestPath string) (*snapshot.Manifest, error) {
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

func NewManifestRepository(repositoryPath string, client *sftp.Client) (*ManifestRepository, error) {
	return &ManifestRepository{repositoryPath: repositoryPath, client: client}, nil
}
