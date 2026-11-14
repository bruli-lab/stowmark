package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
)

const IgnoreFileName = snapshot.IgnoreFileName

type SkippedFile struct {
	Path string
	Kind string
}

type CreateResult struct {
	ID           string
	FileCount    int
	TotalSize    int64
	SkippedFiles []SkippedFile
}

func (h *Handler) CreateSnapshot(ctx context.Context, source string, privateKey *string, exclude ...string) (*CreateResult, error) {
	obsv, err := buildObservability(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	evtr := app.NewEventsTracing()
	evtr.Add(snapshot.SnapshotCreatedEventName, app.NewCreateSnapshotTracer(obsv.Logger))
	mdw, err := middlewares.BuildCommandMiddlewares(obsv, evtr)
	if err != nil {
		return nil, err
	}
	handler := mdw(app.NewCreateSnapshot(
		h.sourceRepository,
		h.manifestRepository,
		h.objectRepository,
		h.folderRepository,
		h.asymmetricKeyPairRepository,
		h.symmetricKeyRepository,
		h.locker,
	))

	events, err := handler.Handle(ctx, app.CreateSnapshotCommand{
		RepositoryPath: h.repositoryPath,
		SourcePath:     source,
		PrivateKey:     privateKey,
		Exclude:        exclude,
	})
	if err != nil {
		return nil, err
	}
	result, err := app.SingleEvent[*snapshot.SnapshotCreated](events)
	if err != nil {
		return nil, err
	}
	skipped := make([]SkippedFile, len(result.SkippedFiles()))
	for i, file := range result.SkippedFiles() {
		skipped[i] = SkippedFile{Path: file.Path(), Kind: file.Kind()}
	}
	return &CreateResult{
		ID:           result.SnapshotID(),
		FileCount:    result.FileCount(),
		TotalSize:    result.TotalSize(),
		SkippedFiles: skipped,
	}, nil
}
