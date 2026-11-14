package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/google/uuid"
)

const InitCommandName = "init"

type InitCommand struct {
	RepositoryPath string
	Compression    *repository.Compression
	FormatVersion  repository.FormatVersion
	PublicKeyPath  *string
	Force          bool
}

func (i InitCommand) Name() string {
	return InitCommandName
}

type Init struct {
	folderRepo FolderRepository
	keys       keyring
	locker     *RepositoryLocker
}

func (i *Init) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(InitCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(InitCommandName, cmd.Name())
	}
	repo, err := i.newRepository(ctx, co)
	if err != nil {
		return nil, err
	}
	exists, err := i.folderRepo.Exists(ctx, repo.Path())
	if err != nil {
		return nil, err
	}
	if err := i.folderRepo.CreateFolder(ctx, repo.Path()); err != nil {
		return nil, err
	}
	var (
		repositoryID string
		warnings     []string
	)
	err = i.locker.WithLock(ctx, repository.ExclusiveLock, func(ctx context.Context) error {
		conf, initWarnings, err := i.writeRepository(ctx, repo, exists, co.Force)
		if err != nil {
			return err
		}
		repositoryID = conf.ID().String()
		warnings = initWarnings
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []event.Event{
		NewInitEvent(repositoryID, warnings),
	}, nil
}

func (i *Init) writeRepository(ctx context.Context, repo *repository.Repository, exists, force bool) (conf *repository.Config, warnings []string, err error) {
	conf = repo.Config()
	if exists {
		previous, err := i.folderRepo.GetConfig(ctx, repo.Path())
		if err != nil {
			return nil, nil, err
		}
		conf, warnings = previous.Reinitialize(conf, force)
	}
	if err := i.folderRepo.CreateConfig(ctx, repo.Path(), conf); err != nil {
		return nil, nil, err
	}
	if err := i.folderRepo.CreateFolder(ctx, repo.ObjectsFolder()); err != nil {
		return nil, nil, err
	}
	if err := i.folderRepo.CreateFolder(ctx, repo.SnapshotsFolder()); err != nil {
		return nil, nil, err
	}
	return conf, warnings, nil
}

func (i *Init) newRepository(ctx context.Context, co InitCommand) (*repository.Repository, error) {
	var encryptionConfig *encryption.EncryptionConfig
	if co.PublicKeyPath != nil {
		created, err := i.keys.newEncryptionConfig(ctx, *co.PublicKeyPath)
		if err != nil {
			return nil, err
		}
		encryptionConfig = created
	}
	conf := repository.NewConfig(uuid.New(), co.FormatVersion, co.Compression, encryptionConfig)
	return repository.NewRepository(co.RepositoryPath, conf)
}

func NewInit(
	folderRepo FolderRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
) *Init {
	return &Init{folderRepo: folderRepo, keys: newKeyring(folderRepo, asymmetricRepo, symmetricRepo), locker: locker}
}

type InitEvent struct {
	event.BasicEvent
	RepositoryID string
	Warnings     []string
}

func NewInitEvent(repositoryID string, warnings []string) *InitEvent {
	return &InitEvent{
		BasicEvent:   event.NewBasicEvent(InitCommandName, uuid.New(), repositoryID),
		RepositoryID: repositoryID,
		Warnings:     warnings,
	}
}
