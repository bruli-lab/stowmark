package repositories

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"

	"github.com/bruli-lab/stowmark/internal/infra/blob"

	"cloud.google.com/go/storage"
	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/bruli-lab/stowmark/internal/infra/gcs"
)

type GCSRepositories struct {
	repositoryPath string
	client         *storage.Client
	bucket         string
}

func (r GCSRepositories) FolderRepository() app.FolderRepository {
	return gcs.NewFolderRepositoryRepository(r.client, r.bucket)
}

func (r GCSRepositories) RepositoryPath() string {
	return r.repositoryPath
}

func (r GCSRepositories) Close() error {
	return r.client.Close()
}

func (r GCSRepositories) BlobStore() (blob.Store, error) {
	return gcs.NewBlobStore(r.client, r.bucket, r.repositoryPath), nil
}

func NewGCSRepositories(ctx context.Context, address string, environment map[string]string) (*GCSRepositories, error) {
	conf := config.NewGCSConfigFrom(environment)
	client, err := gcs.NewClient(ctx, conf.Endpoint)
	if err != nil {
		return nil, err
	}
	bucket, err := gcs.ParseBucket(address)
	if err != nil {
		return nil, err
	}
	return &GCSRepositories{repositoryPath: address, client: client, bucket: bucket}, nil
}
