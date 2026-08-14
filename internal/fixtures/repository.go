package fixtures

import (
	"testing"

	"github.com/bruli-lab/go-core/fixtures"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type RepositoryBuilder struct {
	Path   *string
	Config *repository.Config
}

func (b RepositoryBuilder) Build(t *testing.T) repository.Repository {
	path := fixtures.SetData("name", b.Path)
	config := fixtures.SetData(ConfigBuilder{}.Build(t), b.Config)
	repo, err := repository.NewRepository(path, &config)
	require.NoError(t, err)
	return *repo
}

type ConfigBuilder struct {
	ID            *uuid.UUID
	FormatVersion *int
}

func (c ConfigBuilder) Build(t *testing.T) repository.Config {
	id := fixtures.SetData(uuid.New(), c.ID)
	compType, err := repository.NewCompression(repository.NoneCompressionType, nil)
	require.NoError(t, err)
	version := fixtures.SetData(repository.CurrentFormatVersion, c.FormatVersion)
	co := repository.NewConfig(id, version, compType)
	return *co
}
