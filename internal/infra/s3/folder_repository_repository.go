package s3

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/google/uuid"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/aws/smithy-go"
)

type FolderRepositoryRepository struct {
	client *s3.Client
	bucket string
}

func (f FolderRepositoryRepository) Exists(ctx context.Context, repositoryPath string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	key := strings.TrimPrefix(
		path.Join(repositoryPath, model.ConfigFile),
		"/",
	)

	_, err := f.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(f.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}

	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return false, nil
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey", "NoSuchBucket":
			return false, nil
		}
	}

	return false, fmt.Errorf(
		"check repository config %q in bucket %q: %w",
		key,
		f.bucket,
		err,
	)
}

func (f FolderRepositoryRepository) CreateFolder(ctx context.Context, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (f FolderRepositoryRepository) CreateConfig(ctx context.Context, repositoryPath string, c *repository.Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	co := model.Config{
		ID:            c.Id().String(),
		FormatVersion: c.FormatVersion(),
		CreatedAt:     c.CreatedAt().In(time.Local).Format(time.RFC3339),
		Compression: model.Compression{
			Type:  c.Compression().CompType().String(),
			Level: c.Compression().Level(),
		},
	}

	data, err := json.MarshalIndent(co, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	data = append(data, '\n')

	key := path.Join(repositoryPath, model.ConfigFile)
	_, err = f.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(f.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf(
			"write config %q in bucket %q: %w",
			key,
			f.bucket,
			err,
		)
	}
	return nil
}

func (f FolderRepositoryRepository) GetConfig(ctx context.Context, repositoryPath string) (*repository.Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := path.Join(repositoryPath, model.ConfigFile)

	output, err := f.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(f.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var apiErr smithy.APIError

		if errors.As(err, &apiErr) {
			switch apiErr.ErrorCode() {
			case "NoSuchKey", "NotFound", "NoSuchBucket":
				return nil, repository.NewNotFoundError(
					"config file not found",
				)
			}
		}

		return nil, fmt.Errorf(
			"get config %q from bucket %q: %w",
			key,
			f.bucket,
			err,
		)
	}
	defer func() {
		_ = output.Body.Close()
	}()

	data, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, fmt.Errorf(
			"read config %q from bucket %q: %w",
			key,
			f.bucket,
			err,
		)
	}

	var conf model.Config
	if err := json.Unmarshal(data, &conf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	id, err := uuid.Parse(conf.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config id: %w", err)
	}

	compType, err := repository.ParseCompressionType(
		conf.Compression.Type,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to parse compression type: %w",
			err,
		)
	}

	comp, err := repository.NewCompression(
		*compType,
		conf.Compression.Level,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create compression: %w",
			err,
		)
	}

	return repository.NewConfig(id, comp), nil
}

func NewFolderRepositoryRepository(client *s3.Client, bucket string) *FolderRepositoryRepository {
	return &FolderRepositoryRepository{client: client, bucket: bucket}
}
