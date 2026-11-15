package stowmark

import (
	"context"
	"log/slog"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

type Compression struct {
	Type  string
	Level *int
}

type ObjectLockMode = repository.ObjectLockMode

const (
	GovernanceObjectLock = repository.GovernanceObjectLock
	ComplianceObjectLock = repository.ComplianceObjectLock
)

var (
	ErrObjectLockUnsupported = repository.ErrObjectLockUnsupported
	ErrObjectLockDisabled    = repository.ErrObjectLockDisabled
)

type ObjectLock struct {
	Mode   ObjectLockMode
	Period time.Duration
}

type InitOptions struct {
	Compression   *Compression
	FormatVersion int
	PublicKey     *string
	ObjectLock    *ObjectLock
	Force         bool
}

func (h *Handler) Init(ctx context.Context, comp *Compression, formatVersion int, publicKey *string, force bool) error {
	return h.InitWithOptions(ctx, InitOptions{Compression: comp, FormatVersion: formatVersion, PublicKey: publicKey, Force: force})
}

func (h *Handler) InitWithOptions(ctx context.Context, options InitOptions) error {
	comp := options.Compression
	if comp == nil {
		comp = &Compression{Type: repository.NoneCompressionType.String()}
	}
	compType, err := repository.ParseCompressionType(comp.Type)
	if err != nil {
		return err
	}
	compre, err := repository.NewCompression(*compType, comp.Level)
	if err != nil {
		return err
	}
	version, err := repository.ParseFormatVersionOrDefault(options.FormatVersion)
	if err != nil {
		return err
	}
	lock, err := options.ObjectLock.build()
	if err != nil {
		return err
	}

	events, err := h.repository.Init().Handle(ctx, app.InitCommand{
		RepositoryPath: h.repository.Path(),
		Compression:    compre,
		FormatVersion:  version,
		PublicKeyPath:  options.PublicKey,
		ObjectLock:     lock,
		Force:          options.Force,
	})
	if err != nil {
		return err
	}
	result, err := app.SingleEvent[*repository.RepositoryInitialized](events)
	if err != nil {
		return err
	}
	for _, warning := range result.Warnings() {
		slog.Warn(warning)
	}
	return nil
}

func (o *ObjectLock) build() (*repository.ObjectLock, error) {
	if o == nil {
		return nil, nil
	}
	mode := o.Mode
	if mode == "" {
		mode = GovernanceObjectLock
	}
	return repository.NewObjectLock(mode, o.Period)
}
