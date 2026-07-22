package fixtures

import (
	"github.com/bruli-lab/go-core/fixtures"
	"github.com/bruli-lab/stonekeep.git/internal/domain/snapshot"
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
}

func (b FileBuilder) Build() snapshot.File {
	path := fixtures.SetData("path", b.Path)
	size := fixtures.SetData(20, b.Size)
	fi := snapshot.NewFile(path, size)
	return *fi
}
