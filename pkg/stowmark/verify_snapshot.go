package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
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
	decryptSvc := encryption.NewDecryptSymmetricKey(h.symmetricKeyRepository, h.asymmetricKeyPaiRepository)
	svc := snapshot.NewVerifier(h.objectRepository, h.manifestRepository, h.folderRepository, decryptSvc)
	result, err := svc.Verify(ctx, h.repositoryPath, id, privateKey)
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
