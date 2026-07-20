package repository

import (
	"errors"
	"fmt"
)

const (
	ObjectsFolder   = "objects"
	SnapshotsFolder = "snapshots"
)

var (
	ErrInvalidRepositoryName    = errors.New("invalid repository name")
	ErrMissingCRepositoryConfig = errors.New("missing repository configuration")
)

type Repository struct {
	name   string
	config *Config
}

func (r Repository) Name() string {
	return r.name
}

func (r Repository) Config() *Config {
	return r.config
}

func (r Repository) SnapshotsFolder() string {
	return fmt.Sprintf("%s/%s", r.name, SnapshotsFolder)
}

func (r Repository) ObjectsFolder() string {
	return fmt.Sprintf("%s/%s", r.name, ObjectsFolder)
}

func (r Repository) validate() error {
	switch {
	case r.name == "":
		return ErrInvalidRepositoryName
	case r.config == nil:
		return ErrMissingCRepositoryConfig
	default:
	}
	return nil
}

func NewRepository(name string, config *Config) (*Repository, error) {
	r := Repository{name: name, config: config}
	if err := r.validate(); err != nil {
		return nil, err
	}
	return &r, nil
}
