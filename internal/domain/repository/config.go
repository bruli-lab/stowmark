package repository

import (
	"time"

	"github.com/google/uuid"
)

const DefaultFormatVersion = 1

type Config struct {
	id            uuid.UUID
	formatVersion int
	createdAt     time.Time
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

func NewConfig(id uuid.UUID, createdAt time.Time) *Config {
	return &Config{id: id, formatVersion: DefaultFormatVersion, createdAt: createdAt}
}
