package cli

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	s3infra "github.com/bruli-lab/stowmark/internal/infra/s3"
)

type S3Repositories struct {
	client                 *s3.Client
	repositoryPath, bucket string
}

func (s S3Repositories) FolderRepository() repository.FolderRepository {
	return s3infra.NewFolderRepositoryRepository(s.client, s.bucket)
}

func (s S3Repositories) RepositoryPath() string {
	return s.repositoryPath
}

func (s S3Repositories) ObjectRepository() (snapshot.ObjectRepository, error) {
	return s3infra.NewObjectRepository(s.client, s.bucket, s.repositoryPath), nil
}

func (s S3Repositories) ManifestRepository() (snapshot.ManifestRepository, error) {
	return s3infra.NewManifestRepository(s.client, s.bucket, s.repositoryPath), nil
}

func (s S3Repositories) Close() error {
	return nil
}

func NewS3Repositories(ctx context.Context, repositoryPath string) (*S3Repositories, error) {
	cfg, err := config.NewS3Config()
	if err != nil {
		return nil, err
	}
	storage, err := s3infra.ParseS3URL(repositoryPath)
	if err != nil {
		return nil, err
	}
	client, err := s3infra.NewS3Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &S3Repositories{repositoryPath: storage.Prefix, client: client, bucket: storage.Bucket}, nil
}
