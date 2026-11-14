package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
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
	result, err := app.QueryResult[*snapshot.Result](h.repository.VerifySnapshot().Handle(ctx, app.VerifySnapshotQuery{
		SnapshotID:     id,
		RepositoryPath: h.repository.Path(),
		PrivateKeyPath: privateKey,
	}))
	if err != nil {
		return nil, err
	}
	return newResult(result.SnapshotID(), result.TotalFiles(), result.Failed(), result.IsSuccess()), nil
}

func newResult(snapshotID string, totalFiles int, results []snapshot.FailedResult, isSuccess bool) *Result {
	failed := make([]FailedResult, len(results))
	for i, f := range results {
		failed[i] = FailedResult{Path: f.Path(), Reason: f.Reason()}
	}
	return &Result{SnapshotID: snapshotID, TotalFiles: totalFiles, Failed: failed, IsSuccess: isSuccess}
}
