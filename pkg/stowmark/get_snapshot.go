package stowmark

import (
	"context"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

type Chunk struct {
	Hash   string
	Size   int64
	Offset int64
}
type SnapshotFile struct {
	Path   string
	Hash   string
	Size   int64
	Chunks []Chunk
}

type Snapshot struct {
	ID        string
	CreatedAt time.Time
	Source    string
	Files     []SnapshotFile
}

func (h *Handler) GetSnapshot(ctx context.Context, id string) (*Snapshot, error) {
	manifest, err := snapshot.NewGetManifest(h.manifestRepository).Get(ctx, id)
	if err != nil {
		return nil, err
	}

	files := make([]SnapshotFile, len(manifest.Files()))
	for i, file := range manifest.Files() {
		chunks := make([]Chunk, len(file.Chunks()))
		for j, chunk := range file.Chunks() {
			chunks[j] = Chunk{
				Hash:   chunk.Hash(),
				Size:   chunk.Size(),
				Offset: chunk.Offset(),
			}
		}
		files[i] = SnapshotFile{
			Path:   file.Path(),
			Hash:   file.Hash(),
			Size:   file.Size(),
			Chunks: chunks,
		}
	}

	return &Snapshot{
		ID:        manifest.Id(),
		CreatedAt: manifest.CreatedAt(),
		Source:    manifest.Source(),
		Files:     files,
	}, nil
}
