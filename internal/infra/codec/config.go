package codec

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/model"
)

func MarshalConfig(ctx context.Context, c *repository.Config) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	co := model.Config{
		ID:            c.Id().String(),
		FormatVersion: c.FormatVersion().Int(),
		CreatedAt:     c.CreatedAt().In(time.Local).Format(time.RFC3339),
		Compression: model.Compression{
			Type:  c.Compression().CompType().String(),
			Level: c.Compression().Level(),
		},
	}

	data, err := json.MarshalIndent(co, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	data = append(data, '\n')
	return data, nil
}
