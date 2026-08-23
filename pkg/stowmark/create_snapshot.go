package stowmark

import (
	"context"
	"errors"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
)

type CreateResult struct {
	ID        string
	FileCount int
	TotalSize int64
}

func (h *Handler) CreateSnapshot(ctx context.Context, source string, privateKey *string) (*CreateResult, error) {
	obsv, err := builtObservability(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	svc := snapshot.NewCreate(
		h.sourceRepository,
		h.manifestRepository,
		h.objectRepository,
		repository.NewGetConfig(h.folderRepository),
		encryption.NewDecryptSymmetricKey(h.symmetricKeyRepository, h.asymmetricKeyPaiRepository),
	)
	mdw, err := middlewares.BuildCommandMiddlewares(obsv)
	if err != nil {
		return nil, err
	}
	handler := mdw(app.NewCreateSnapshot(svc))

	events, err := handler.Handle(ctx, app.CreateSnapshotCommand{
		RepositoryPath: h.repositoryPath,
		SourcePath:     source,
		PrivateKey:     privateKey,
	})
	if err != nil {
		return nil, err
	}
	if len(events) != 1 {
		return nil, errors.New("unexpected number of events")
	}
	result, ok := events[0].(*app.CreateSnapshotEvent)
	if !ok {
		return nil, errors.New("unexpected event type")
	}
	return &CreateResult{
		ID:        result.SnapshotID,
		FileCount: result.FileCount,
		TotalSize: result.TotalSize,
	}, nil
}
