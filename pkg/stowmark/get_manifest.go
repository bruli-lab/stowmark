package stowmark

import (
	"context"
	"fmt"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
)

type Chunk struct {
	Hash   string
	Size   int64
	Offset int64
}
type ManifestFile struct {
	Path   string
	Hash   string
	Size   int64
	Chunks []Chunk
}

type Manifest struct {
	ID        string
	CreatedAt time.Time
	Source    string
	Files     []ManifestFile
}

func (h *Handler) GetManifest(ctx context.Context, id string) (*Manifest, error) {
	obsv, err := builtObservability(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	svc := snapshot.NewGetManifest(h.manifestRepository)
	mdw, err := middlewares.BuildQueryMiddlewares(obsv)
	if err != nil {
		return nil, err
	}
	handler := mdw(app.NewManifestGet(svc))
	result, err := handler.Handle(ctx, app.ManifestGetQuery{
		SnapshotID: id,
	})
	if err != nil {
		return nil, err
	}
	manifest, ok := result.(*snapshot.Manifest)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
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
		}
	}

	return &Manifest{
		ID:        manifest.Id(),
		CreatedAt: manifest.CreatedAt(),
		Source:    manifest.Source(),
		Files:     files,
	}, nil
}
