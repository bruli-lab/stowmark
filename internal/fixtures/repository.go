package fixtures

import (
	"testing"

	"github.com/bruli-lab/go-core/fixtures"
	"github.com/bruli-lab/stowmark.git/internal/domain/repository"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type RepositoryBuilder struct {
	Path   *string
	Config *repository.Config
}

func (b RepositoryBuilder) Build(t *testing.T) repository.Repository {
	path := fixtures.SetData("name", b.Path)
	config := fixtures.SetData(ConfigBuilder{}.Build(), b.Config)
	repo, err := repository.NewRepository(path, &config)
	require.NoError(t, err)
	return *repo
}

type ConfigBuilder struct {
	ID *uuid.UUID
}

func (c ConfigBuilder) Build() repository.Config {
	id := fixtures.SetData(uuid.New(), c.ID)
	co := repository.NewConfig(id, repository.NoneCompression(repository.NoneCompressionType, nil))
	return *co
}
