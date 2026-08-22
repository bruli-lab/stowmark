package stowmark

import (
	"context"
	"log/slog"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/google/uuid"
)

type Compression struct {
	Type  string
	Level *int
}

func (h *Handler) Init(ctx context.Context, comp *Compression, formatVersion int, publicKey *string, force bool) error {
	svc := repository.NewInit(h.folderRepository)
	compType, err := repository.ParseCompressionType(comp.Type)
	if err != nil {
		return err
	}
	compre, err := repository.NewCompression(*compType, comp.Level)
	if err != nil {
		return err
	}
	var encryptionConfig *encryption.EncryptionConfig
	if publicKey != nil {
		encryptionConfigSvc := encryption.NewCreateEncryptionConfig(h.asymmetricKeyPaiRepository, h.symmetricKeyRepository)
		encryptionConfig, err = encryptionConfigSvc.Create(ctx, *publicKey)
		if err != nil {
			return err
		}
	}
	conf := repository.NewConfig(uuid.New(), repository.ParseFormatVersion(formatVersion), compre, encryptionConfig)
	repo, err := repository.NewRepository(h.repositoryPath, conf)
	if err != nil {
		return err
	}
	result, err := svc.Do(ctx, repo, force)
	if err != nil {
		return err
	}
	for _, warning := range result.Warnings {
		slog.Warn(warning)
	}
	return nil
}
