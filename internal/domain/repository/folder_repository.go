package repository

import "context"

//go:generate go tool moq -out folder_repository_mock.go . FolderRepository
type FolderRepository interface {
	Exists(ctx context.Context, name string) (bool, error)
	CreateFolder(ctx context.Context, name string) error
	CreateConfig(ctx context.Context, path string, c *Config) error
}
