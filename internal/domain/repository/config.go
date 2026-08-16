package repository

import (
	"time"

	"github.com/google/uuid"
)

type Config struct {
	id            uuid.UUID
	formatVersion FormatVersion
	createdAt     time.Time
	compression   *Compression
}

func (c Config) Compression() *Compression {
	return c.compression
}

func (c Config) Id() uuid.UUID {
	return c.id
}

func (c Config) FormatVersion() FormatVersion {
	return c.formatVersion
}

func (c Config) CreatedAt() time.Time {
	return c.createdAt
}

func NewConfig(id uuid.UUID, version FormatVersion, comp *Compression) *Config {
	return &Config{id: id, formatVersion: version, createdAt: time.Now().UTC(), compression: comp}
}
