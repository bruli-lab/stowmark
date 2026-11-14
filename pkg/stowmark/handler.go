package stowmark

import (
	"context"
	"errors"

	"github.com/bruli-lab/stowmark/internal/bootstrap"
	"github.com/bruli-lab/stowmark/internal/config"
)

type Handler struct {
	runtime    *bootstrap.Runtime
	repository *bootstrap.Repository
}

func (h *Handler) Close() error {
	return errors.Join(h.repository.Close(), h.runtime.Close(context.Background()))
}

func NewHandler(ctx context.Context, repositoryPath string) (*Handler, error) {
	runtime, err := bootstrap.NewRuntime(ctx, config.ProcessEnvironment())
	if err != nil {
		return nil, err
	}
	repo, err := runtime.OpenRepository(ctx, repositoryPath)
	if err != nil {
		return nil, errors.Join(err, runtime.Close(ctx))
	}
	return &Handler{runtime: runtime, repository: repo}, nil
}
