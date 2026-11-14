package stowmark

import (
	"context"
	"errors"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/bootstrap"
	"github.com/bruli-lab/stowmark/internal/config"
)

var ErrPrivateKeyRequired = app.ErrPrivateKeyRequired

type Handler struct {
	runtime    *bootstrap.Runtime
	repository *bootstrap.Repository
	privateKey *string
	progress   app.Progress
}

type Option func(*Handler)

type (
	Progress      = app.Progress
	ProgressStage = app.ProgressStage
)

const (
	ProgressBackingUp   = app.ProgressBackingUp
	ProgressRestoring   = app.ProgressRestoring
	ProgressVerifying   = app.ProgressVerifying
	ProgressReadingData = app.ProgressReadingData
	ProgressRekeying    = app.ProgressRekeying
)

func WithProgress(progress Progress) Option {
	return func(h *Handler) {
		h.progress = progress
	}
}

func WithPrivateKey(path string) Option {
	return func(h *Handler) {
		h.privateKey = &path
	}
}

func (h *Handler) Close() error {
	return errors.Join(h.repository.Close(), h.runtime.Close(context.Background()))
}

func (h *Handler) key(explicit *string) *string {
	if explicit != nil {
		return explicit
	}
	return h.privateKey
}

func NewHandler(ctx context.Context, repositoryPath string, options ...Option) (*Handler, error) {
	runtime, err := bootstrap.NewRuntime(ctx, config.ProcessEnvironment())
	if err != nil {
		return nil, err
	}
	repo, err := runtime.OpenRepository(ctx, repositoryPath)
	if err != nil {
		return nil, errors.Join(err, runtime.Close(ctx))
	}
	handler := &Handler{runtime: runtime, repository: repo}
	for _, option := range options {
		option(handler)
	}
	return handler, nil
}
