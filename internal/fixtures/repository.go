package fixtures

import (
	"testing"

	"github.com/bruli-lab/go-core/fixtures"
	"github.com/bruli-lab/stowmark.git/internal/domain/repository"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type RepositoryBuilder struct {
	Name   *string
	Config *repository.Config
}

func (b RepositoryBuilder) Build(t *testing.T) repository.Repository {
	name := fixtures.SetData("name", b.Name)
	config := fixtures.SetData(ConfigBuilder{}.Build(), b.Config)
	repo, err := repository.NewRepository(name, &config)
	require.NoError(t, err)
	return *repo
}

type ConfigBuilder struct {
	ID *uuid.UUID
}

func (c ConfigBuilder) Build() repository.Config {
	id := fixtures.SetData(uuid.New(), c.ID)
	co := repository.NewConfig(id, repository.NoneCompression())
	return *co
}
