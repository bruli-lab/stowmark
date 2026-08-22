package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
)

func (h *Handler) RestoreSnapshot(ctx context.Context, snapshotID string, destinationPath, privateKey *string) (*Result, error) {
	decryptKeySvc := encryption.NewDecryptSymmetricKey(encrypt.NewSymmetricRepository(), disk.NewAsymmetricKeyPairRepository())
	svc := snapshot.NewRestore(
		h.manifestRepository,
		h.objectRepository,
		h.folderRepository,
		decryptKeySvc,
	)
	result, err := svc.Restore(ctx, snapshotID, h.repositoryPath, destinationPath, privateKey)
	if err != nil {
		return nil, err
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
