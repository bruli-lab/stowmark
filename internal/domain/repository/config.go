package repository

import (
	"time"

	"github.com/google/uuid"
)

const (
	OneFormatVersion = iota + 1

	CurrentFormatVersion = OneFormatVersion
)

type Config struct {
	id            uuid.UUID
	formatVersion int
	createdAt     time.Time
	compression   *Compression
}

func (c Config) Compression() *Compression {
	return c.compression
}

func (c Config) Id() uuid.UUID {
	return c.id
}

func (c Config) FormatVersion() int {
	return c.formatVersion
}

func (c Config) CreatedAt() time.Time {
	return c.createdAt
}

func NewConfig(id uuid.UUID, version int, comp *Compression) *Config {
	return &Config{id: id, formatVersion: version, createdAt: time.Now().UTC(), compression: comp}
}
