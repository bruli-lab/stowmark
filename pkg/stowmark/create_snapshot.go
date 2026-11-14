package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
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
	events, err := h.repository.CreateSnapshot().Handle(ctx, app.CreateSnapshotCommand{
		RepositoryPath: h.repository.Path(),
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
