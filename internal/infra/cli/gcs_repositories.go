package cli

import (
	"context"

	"cloud.google.com/go/storage"
	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/gcs"
)

type GCSRepositories struct {
	repositoryPath string
	client         *storage.Client
	bucket         string
}

func (r GCSRepositories) FolderRepository() repository.FolderRepository {
	return gcs.NewFolderRepositoryRepository(r.client, r.bucket)
}

func (r GCSRepositories) RepositoryPath() string {
	return r.repositoryPath
}

func (r GCSRepositories) ObjectRepository() (snapshot.ObjectRepository, error) {
	// TODO implement me
	panic("implement me")
}

func (r GCSRepositories) ManifestRepository() (snapshot.ManifestRepository, error) {
	// TODO implement me
	panic("implement me")
}

func (r GCSRepositories) Close() error {
	return r.client.Close()
}

func NewGCSRepositories(ctx context.Context, address string) (*GCSRepositories, error) {
	conf := config.NewGCSConfig()
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
