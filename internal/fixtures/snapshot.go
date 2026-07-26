package fixtures

import (
	"time"

	"github.com/bruli-lab/go-core/fixtures"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/google/uuid"
)

type SourceBuilder struct {
	AbsolutePath *string
	Files        []snapshot.File
}

func (b SourceBuilder) Build() snapshot.Source {
	path := fixtures.SetData("path", b.AbsolutePath)
	so := snapshot.NewSource(path, b.Files)
	return *so
}

type FileBuilder struct {
	Path *string
	Size *int64
	Hash *string
}

func (b FileBuilder) Build() snapshot.File {
	path := fixtures.SetData("path", b.Path)
	size := fixtures.SetData(20, b.Size)
	hash := fixtures.SetData(uuid.NewString(), b.Hash)
	fi := snapshot.File{}
	fi.Hydrate(path, hash, size)
	return fi
}

type ManifestBuilder struct {
	ID        *string
	Source    *string
	CreatedAt *time.Time
	Files     []snapshot.File
}

func (b ManifestBuilder) Build() snapshot.Manifest {
	id := fixtures.SetData(uuid.NewString(), b.ID)
	created := fixtures.SetData(time.Now(), b.CreatedAt)
	source := fixtures.SetData("source", b.Source)
	man := snapshot.NewManifest(id, b.Files, created, source)
	return *man
}
