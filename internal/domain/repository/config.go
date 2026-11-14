package repository

import (
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/google/uuid"
)

type Config struct {
	id            uuid.UUID
	formatVersion FormatVersion
	createdAt     time.Time
	compression   *Compression
	encryption    *encryption.EncryptionConfig
}

func (c Config) Encryption() *encryption.EncryptionConfig {
	return c.encryption
}

func (c Config) Compression() *Compression {
	return c.compression
}

func (c Config) ID() uuid.UUID {
	return c.id
}

func (c Config) FormatVersion() FormatVersion {
	return c.formatVersion
}

func (c Config) CreatedAt() time.Time {
	return c.createdAt
}

func NewConfig(
	id uuid.UUID,
	version FormatVersion,
	comp *Compression,
	encryptionConfig *encryption.EncryptionConfig,
) *Config {
	return HydrateConfig(id, version, time.Now().UTC(), comp, encryptionConfig)
}

func HydrateConfig(
	id uuid.UUID,
	version FormatVersion,
	createdAt time.Time,
	comp *Compression,
	encryptionConfig *encryption.EncryptionConfig,
) *Config {
	return &Config{
		id:            id,
		formatVersion: version,
		createdAt:     createdAt,
		compression:   comp,
		encryption:    encryptionConfig,
	}
}

func (c Config) OrphanGenerations(stored []uint64) []uint64 {
	orphans := make([]uint64, 0, len(stored))
	for _, generation := range stored {
		if c.encryption != nil && generation == c.encryption.Generation() {
			continue
		}
		orphans = append(orphans, generation)
	}
	return orphans
}
