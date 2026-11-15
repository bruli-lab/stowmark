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
	var options []func(*awsconfig.LoadOptions) error
	if cfg.Region != nil && *cfg.Region != "" {
		options = append(options, awsconfig.WithRegion(*cfg.Region))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}

	if cfg.Endpoint != nil {
		awsCfg.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired

		awsCfg.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	}

	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if cfg.Endpoint != nil {
			options.BaseEndpoint = cfg.Endpoint
		}

		if cfg.PathStyle != nil {
			options.UsePathStyle = *cfg.PathStyle
		}
	})

	return client, nil
}
