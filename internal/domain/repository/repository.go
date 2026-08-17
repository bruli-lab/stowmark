package repository

import (
	"errors"
	"fmt"
)

const (
	ObjectsFolder   = "objects"
	SnapshotsFolder = "snapshots"
	EncryptedFolder = "encrypted"
)

var (
	ErrInvalidRepositoryName    = errors.New("invalid repository name")
	ErrMissingCRepositoryConfig = errors.New("missing repository configuration")
)

type Repository struct {
	path   string
	config *Config
}

func (r Repository) Path() string {
	return r.path
}

func (r Repository) Config() *Config {
	return r.config
}

func (r Repository) SnapshotsFolder() string {
	return fmt.Sprintf("%s/%s", r.path, SnapshotsFolder)
}

func (r Repository) ObjectsFolder() string {
	return fmt.Sprintf("%s/%s", r.path, ObjectsFolder)
}

func (r Repository) validate() error {
	switch {
	case r.path == "":
		return ErrInvalidRepositoryName
	case r.config == nil:
		return ErrMissingCRepositoryConfig
	default:
	}
	return nil
}

func NewRepository(path string, config *Config) (*Repository, error) {
	r := Repository{path: path, config: config}
	if err := r.validate(); err != nil {
		return nil, err
	}
	return &r, nil
}
