package fixtures

import (
	"testing"
	"time"

	"github.com/bruli-lab/go-core/fixtures"
	"github.com/bruli-lab/stonekeep.git/internal/domain/repository"
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
	ID        *uuid.UUID
	createdAt *time.Time
}

func (c ConfigBuilder) Build() repository.Config {
	id := fixtures.SetData(uuid.New(), c.ID)
	createdAt := fixtures.SetData(time.Now(), c.createdAt)
	co := repository.NewConfig(id, createdAt)
	return *co
}
