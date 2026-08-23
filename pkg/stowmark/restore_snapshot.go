package stowmark

import (
	"context"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
)

func (h *Handler) RestoreSnapshot(ctx context.Context, snapshotID string, destinationPath, privateKey *string) (*Result, error) {
	obsv, err := builtObservability(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	decryptKeySvc := encryption.NewDecryptSymmetricKey(encrypt.NewSymmetricRepository(), disk.NewAsymmetricKeyPairRepository())
	svc := snapshot.NewRestore(
		h.manifestRepository,
		h.objectRepository,
		h.folderRepository,
		decryptKeySvc,
	)
	tracerMdw := app.NewTracerCommandMiddleware(obsv.TracerProvider)
	handler := tracerMdw(app.NewRestoreSnapshot(svc))
	events, err := handler.Handle(ctx, app.RestoreSnapshotCommand{
		SnapshotID:      snapshotID,
		RepositoryPath:  h.repositoryPath,
		PrivateKeyPath:  privateKey,
		DestinationPath: destinationPath,
	})
	if err != nil {
		return nil, err
	}
	if len(events) != 1 {
		return nil, fmt.Errorf("unexpected number of events")
	}
	result, ok := events[0].(*app.RestoreSnapshotEvent)
	if !ok {
		return nil, fmt.Errorf("unexpected event type")
	}
	failed := make([]FailedResult, len(result.FailedFiles))
	for i, f := range result.FailedFiles {
		failed[i] = FailedResult{
			Path:   f.Path,
			Reason: f.Reason,
		}
	}
	return &Result{
		SnapshotID: result.SnapshotID,
		TotalFiles: result.TotalFiles,
		Failed:     failed,
		IsSuccess:  result.IsSuccess,
	}, nil
}
