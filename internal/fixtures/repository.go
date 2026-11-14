package fixtures

import (
	"testing"

	"github.com/bruli-lab/go-core/fixtures"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type ConfigBuilder struct {
	ID               *uuid.UUID
	FormatVersion    *repository.FormatVersion
	EncryptionConfig *encryption.EncryptionConfig
}

func (c ConfigBuilder) Build(t *testing.T) repository.Config {
	id := fixtures.SetData(uuid.New(), c.ID)
	version := fixtures.SetData(repository.CurrentFormatVersion, c.FormatVersion)
	co := repository.NewConfig(id, version, CompressionBuilder{}.Build(t), c.EncryptionConfig)
	return *co
}

type CompressionBuilder struct {
	Type  *repository.CompressionType
	Level *int
}

func (c CompressionBuilder) Build(t *testing.T) *repository.Compression {
	comp, err := repository.NewCompression(fixtures.SetData(repository.NoneCompressionType, c.Type), c.Level)
	require.NoError(t, err)
	return comp
}
