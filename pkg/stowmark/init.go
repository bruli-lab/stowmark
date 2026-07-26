package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/google/uuid"
)

type Compression struct {
	Type  string
	Level *int
}

func (h *Handler) Init(ctx context.Context, source string, comp *Compression) error {
	svc := repository.NewInit(h.folderRepository)
	compType, err := repository.ParseCompressionType(comp.Type)
	if err != nil {
		return err
	}
	conf := repository.NewConfig(uuid.New(), repository.NoneCompression(*compType, comp.Level))
	repo, err := repository.NewRepository(source, conf)
	if err != nil {
		return err
	}
	return svc.Do(ctx, repo)
}
