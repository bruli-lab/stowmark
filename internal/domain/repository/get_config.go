package repository

import "context"

type GetConfig struct {
	repo FolderRepository
}

func (g GetConfig) Get(ctx context.Context, path string) (*Config, error) {
	return g.repo.GetConfig(ctx, path)
}

func NewGetConfig(repo FolderRepository) *GetConfig {
	return &GetConfig{repo: repo}
}
