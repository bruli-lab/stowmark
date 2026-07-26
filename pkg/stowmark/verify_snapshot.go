package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark.git/internal/domain/snapshot"
)

type FailedResult struct {
	Path, Reason string
}

type VerifyResult struct {
	SnapshotID string
	TotalFiles int
	Failed     []FailedResult
	IsSuccess  bool
}

func (h *Handler) VerifySnapshot(ctx context.Context, id string) (*VerifyResult, error) {
	svc := snapshot.NewVerifier(h.objectRepository, h.manifestRepository)
	result, err := svc.Verify(ctx, id)
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

	return &VerifyResult{
		SnapshotID: result.SnapshotID(),
		TotalFiles: result.TotalFiles(),
		Failed:     failed,
		IsSuccess:  result.IsSuccess(),
	}, nil
}
