package gcs

import (
	"context"
	"fmt"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

func NewClient(ctx context.Context, endpoint *string) (*storage.Client, error) {
	options := []option.ClientOption{
		storage.WithJSONReads(),
	}

	if endpoint != nil {
		options = append(
			options,
			option.WithEndpoint(*endpoint),
			option.WithoutAuthentication(),
		)
	}

	client, err := storage.NewClient(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("create GCS client: %w", err)
	}

	return client, nil
}
