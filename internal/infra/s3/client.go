package s3

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bruli-lab/stowmark/internal/config"
)

func NewS3Client(
	ctx context.Context,
	cfg *config.S3Config,
) (*s3.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}

	if cfg.Endpoint != nil {
		awsCfg.ResponseChecksumValidation =
			aws.ResponseChecksumValidationWhenRequired
	}

	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if cfg.Endpoint != nil {
			options.BaseEndpoint = aws.String(*cfg.Endpoint)
		}

		if cfg.PathStyle != nil {
			options.UsePathStyle = *cfg.PathStyle
		}
	})

	return client, nil
}
