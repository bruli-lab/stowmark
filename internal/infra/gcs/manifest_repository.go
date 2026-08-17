package gcs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"google.golang.org/api/iterator"
)

type ManifestRepository struct {
	client         *storage.Client
	bucket         string
	repositoryPath string
}

func (r ManifestRepository) Save(ctx context.Context, m *snapshot.Manifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if m == nil {
		return errors.New("snapshot manifest is required")
	}

	manifest := model.NewManifest(m)

	data, err := json.MarshalIndent(manifest, "", " ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	data = append(data, '\n')

	manifestFile := fmt.Sprintf("%s.json", m.Id())
	objectPath := path.Join(
		r.repositoryPath,
		repository.SnapshotsFolder,
		manifestFile,
	)

	writeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	object := r.client.
		Bucket(r.bucket).
		Object(objectPath)

	writer := object.NewWriter(writeCtx)
	writer.ContentType = "application/json"

	n, err := writer.Write(data)
	if err != nil {
		cancel()

		return fmt.Errorf("write manifest %q in bucket %q: %w", objectPath, r.bucket, err)
	}

	if n != len(data) {
		cancel()

		return fmt.Errorf("write manifest %q in bucket %q: %w", objectPath, r.bucket, io.ErrShortWrite)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("close manifest %q in bucket %q: %w", objectPath, r.bucket, err)
	}

	return nil
}

func (r ManifestRepository) List(ctx context.Context) ([]snapshot.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	snapshotsPrefix := path.Join(
		r.repositoryPath,
		repository.SnapshotsFolder,
	) + "/"

	objects := r.client.
		Bucket(r.bucket).
		Objects(ctx, &storage.Query{
			Prefix: snapshotsPrefix,
		})

	manifests := make([]snapshot.Manifest, 0)

	for {
		attrs, err := objects.Next()
		if errors.Is(err, iterator.Done) {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("list manifests with prefix %q in bucket %q: %w", snapshotsPrefix, r.bucket, err)
		}

		name := strings.TrimPrefix(attrs.Name, snapshotsPrefix)

		if name == "" ||
			strings.Contains(name, "/") ||
			path.Ext(name) != ".json" {
			continue
		}

		reader, err := r.client.
			Bucket(r.bucket).
			Object(attrs.Name).
			NewReader(ctx)
		if err != nil {
			return nil, fmt.Errorf("open manifest %q in bucket %q: %w", attrs.Name, r.bucket, err)
		}

		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()

		if readErr != nil {
			return nil, fmt.Errorf("read manifest %q in bucket %q: %w", attrs.Name, r.bucket, readErr)
		}

		if closeErr != nil {
			return nil, fmt.Errorf("close manifest %q in bucket %q: %w", attrs.Name, r.bucket, closeErr)
		}

		manifest, err := model.BuildManifestDomain(
			data,
			attrs.Name,
		)
		if err != nil {
			return nil, err
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

	if snapshotID == "" {
		return nil, errors.New("snapshot ID is required")
	}

	if path.Base(snapshotID) != snapshotID {
		return nil, fmt.Errorf("invalid snapshot ID %q", snapshotID)
	}

	objectPath := path.Join(
		r.repositoryPath,
		repository.SnapshotsFolder,
		snapshotID+".json",
	)

	reader, err := r.client.
		Bucket(r.bucket).
		Object(objectPath).
		NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, repository.NewNotFoundError("manifest not found")
		}

		return nil, fmt.Errorf("open manifest %q in bucket %q: %w", objectPath, r.bucket, err)
	}

	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()

	if readErr != nil {
		return nil, fmt.Errorf("read manifest %q in bucket %q: %w", objectPath, r.bucket, readErr)
	}

	if closeErr != nil {
		return nil, fmt.Errorf("close manifest %q in bucket %q: %w", objectPath, r.bucket, closeErr)
	}

	manifest, err := model.BuildManifestDomain(data, objectPath)
	if err != nil {
		return nil, err
	}

	return manifest, nil
}

func NewManifestRepository(client *storage.Client, bucket, repositoryPath string) *ManifestRepository {
	return &ManifestRepository{client: client, bucket: bucket, repositoryPath: repositoryPath}
}
