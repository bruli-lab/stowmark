package stowmark

import (
	"context"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
)

type FailedResult struct {
	Path, Reason string
}

type Result struct {
	SnapshotID string
	TotalFiles int
	Failed     []FailedResult
	IsSuccess  bool
}

func (h *Handler) VerifySnapshot(ctx context.Context, id string, privateKey *string) (*Result, error) {
	obsv, err := builtObservability(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	decryptSvc := encryption.NewDecryptSymmetricKey(h.symmetricKeyRepository, h.asymmetricKeyPaiRepository)
	svc := snapshot.NewVerifier(h.objectRepository, h.manifestRepository, h.folderRepository, decryptSvc)
	mdw, err := middlewares.BuildQueryMiddlewares(obsv)
	if err != nil {
		return nil, err
	}
	handler := mdw(app.NewVerifySnapshot(svc))
	resultQuery, err := handler.Handle(ctx, app.VerifySnapshotQuery{
		SnapshotID:     id,
		RepositoryPath: h.repositoryPath,
		PrivateKeyPath: privateKey,
	})
	if err != nil {
		return nil, err
	}
	result, ok := resultQuery.(*snapshot.Result)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", resultQuery)
	}
	failed := make([]FailedResult, len(result.Failed()))
	for i, f := range result.Failed() {
		failed[i] = FailedResult{
			Path:   f.Path(),
			Reason: f.Reason(),
		}
	}

	return &Result{
		SnapshotID: result.SnapshotID(),
		TotalFiles: result.TotalFiles(),
		Failed:     failed,
		IsSuccess:  result.IsSuccess(),
	}, nil
}
