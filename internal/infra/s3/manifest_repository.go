package s3

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/model"
)

type ManifestRepository struct {
	client         *s3.Client
	bucket         string
	repositoryPath string
}

func (r ManifestRepository) Save(ctx context.Context, m *snapshot.Manifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	manifest := model.NewManifest(m)

	data, err := json.MarshalIndent(manifest, "", " ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	data = append(data, '\n')

	manifestFile := fmt.Sprintf("%s.json", m.Id())
	key := path.Join(
		r.repositoryPath,
		repository.SnapshotsFolder,
		manifestFile,
	)

	_, err = r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(r.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf(
			"write manifest %q in bucket %q: %w",
			key,
			r.bucket,
			err,
		)
	}

	return nil
}

func (r ManifestRepository) List(ctx context.Context) ([]snapshot.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := path.Join(
		r.repositoryPath,
		repository.SnapshotsFolder,
	) + "/"

	paginator := s3.NewListObjectsV2Paginator(
		r.client,
		&s3.ListObjectsV2Input{
			Bucket: aws.String(r.bucket),
			Prefix: aws.String(prefix),
		},
	)

	manifests := make([]snapshot.Manifest, 0)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf(
				"list manifests under %q in bucket %q: %w",
				prefix,
				r.bucket,
				err,
			)
		}

		for _, object := range page.Contents {
			if object.Key == nil {
				continue
			}

			key := *object.Key

			manifest, err := r.getManifest(ctx, key)
			if err != nil {
				return nil, err
			}

			manifests = append(manifests, *manifest)
		}
	}

	slices.SortFunc(
		manifests,
		func(a, b snapshot.Manifest) int {
			return b.CreatedAt().Compare(a.CreatedAt())
		},
	)

	return manifests, nil
}

func (r ManifestRepository) getManifest(ctx context.Context, key string) (*snapshot.Manifest, error) {
	output, err := r.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFoundError(err) {
			return nil, repository.NewNotFoundError("manifest not found")
		}

		return nil, fmt.Errorf("get manifest %q from bucket %q: %w", key, r.bucket, err)
	}

	data, readErr := io.ReadAll(output.Body)
	closeErr := output.Body.Close()

	if readErr != nil {
		return nil, fmt.Errorf("read manifest %q from bucket %q: %w", key, r.bucket, readErr)
	}

	if closeErr != nil {
		return nil, fmt.Errorf("close manifest %q response body: %w", key, closeErr)
	}

	manifest, err := model.BuildManifestDomain(data, key)
	if err != nil {
		return nil, err
	}

	return manifest, nil
}

func (r ManifestRepository) Get(ctx context.Context, snapshotID string) (*snapshot.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	manifestFile := fmt.Sprintf("%s.json", snapshotID)

	key := path.Join(
		r.repositoryPath,
		repository.SnapshotsFolder,
		manifestFile,
	)

	manifest, err := r.getManifest(ctx, key)
	if err != nil {
		return nil, err
	}

	return manifest, nil
}

func NewManifestRepository(client *s3.Client, bucket, repositoryPath string) *ManifestRepository {
	return &ManifestRepository{client: client, bucket: bucket, repositoryPath: repositoryPath}
}

func isNotFoundError(err error) bool {
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}

	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return true
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "NoSuchBucket":
			return true
		}
	}

	return false
}
