package stowmark

import (
	"context"
	"io/fs"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

type Chunk struct {
	Hash   string
	Size   int64
	Offset int64
}
type EntryKind string

const (
	RegularFile EntryKind = "file"
	Directory   EntryKind = "dir"
	Symlink     EntryKind = "symlink"
)

type ManifestFile struct {
	Path    string
	Hash    string
	Size    int64
	Chunks  []Chunk
	Kind    EntryKind
	Target  string
	Mode    fs.FileMode
	ModTime time.Time
}

type Manifest struct {
	ID        string
	CreatedAt time.Time
	Source    string
	Files     []ManifestFile
}

func (h *Handler) GetManifest(ctx context.Context, id string) (*Manifest, error) {
	manifest, err := app.QueryResult[*snapshot.Manifest](h.repository.ManifestGet().Handle(ctx, app.ManifestGetQuery{
		RepositoryPath: h.repository.Path(),
		PrivateKeyPath: h.privateKey,
		SnapshotID:     id,
	}))
	if err != nil {
		return nil, err
	}

	files := make([]ManifestFile, len(manifest.Files()))
	for i, file := range manifest.Files() {
		chunks := make([]Chunk, len(file.Chunks()))
		for j, chunk := range file.Chunks() {
			chunks[j] = Chunk{
				Hash:   chunk.Hash(),
				Size:   chunk.Size(),
				Offset: chunk.Offset(),
			}
		}
		files[i] = ManifestFile{
			Path:   file.Path(),
			Hash:   file.Hash(),
			Size:   file.Size(),
			Chunks: chunks,
			Kind:   EntryKind(file.Kind()),
			Target: file.Target(),
		}
		if metadata, ok := file.Metadata(); ok {
			files[i].Mode = metadata.Mode()
			files[i].ModTime = metadata.ModTime()
		}
	}

	return &Manifest{
		ID:        manifest.ID(),
		CreatedAt: manifest.CreatedAt(),
		Source:    manifest.Source(),
		Files:     files,
	}, nil
}
