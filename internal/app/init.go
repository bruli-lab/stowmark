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
	ObjectLock     *repository.ObjectLock
	Force          bool
}

func (i InitCommand) Name() string {
	return InitCommandName
}

type Init struct {
	folderRepo    FolderRepository
	retentionRepo RetentionRepository
	keys          keyring
	locker        *RepositoryLocker
}

func (i *Init) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(InitCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(InitCommandName, cmd.Name())
	}
	if co.ObjectLock != nil {
		if err := i.retentionRepo.VerifyObjectLock(ctx); err != nil {
			return nil, err
		}
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
	var initialized *repository.RepositoryInitialized
	err = i.locker.WithLock(ctx, repository.ExclusiveLock, func(ctx context.Context) error {
		conf, warnings, err := i.writeRepository(ctx, repo, exists, co)
		if err != nil {
			return err
		}
		initialized = conf.Initialized(warnings)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []event.Event{initialized}, nil
}

func (i *Init) writeRepository(ctx context.Context, repo *repository.Repository, exists bool, co InitCommand) (conf *repository.Config, warnings []string, err error) {
	conf = repo.Config()
	if exists {
		previous, err := i.folderRepo.GetConfig(ctx, repo.Path())
		if err != nil {
			return nil, nil, err
		}
		conf, warnings = previous.Reinitialize(conf, co.Force)
	}
	if err := i.ensureMetadataKey(ctx, conf, co.PublicKeyPath); err != nil {
		return nil, nil, err
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
		created, err := i.keys.newEncryptionConfig(ctx, *co.PublicKeyPath, co.FormatVersion)
		if err != nil {
			return nil, err
		}
		encryptionConfig = created
	}
	conf := repository.NewConfig(uuid.New(), co.FormatVersion, co.Compression, encryptionConfig).WithObjectLock(co.ObjectLock)
	return repository.NewRepository(co.RepositoryPath, conf)
}

func (i *Init) ensureMetadataKey(ctx context.Context, conf *repository.Config, publicKeyPath *string) error {
	encryptionConf := conf.Encryption()
	if encryptionConf == nil || encryptionConf.HasMetadataKey() || !conf.FormatVersion().ContentAddressed() {
		return nil
	}
	if publicKeyPath == nil {
		return ErrMissingMetadataKey
	}
	return i.keys.addMetadataKey(ctx, *publicKeyPath, encryptionConf)
}

func NewInit(
	folderRepo FolderRepository,
	retentionRepo RetentionRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
) *Init {
	return &Init{
		folderRepo:    folderRepo,
		retentionRepo: retentionRepo,
		keys:          newKeyring(folderRepo, asymmetricRepo, symmetricRepo),
		locker:        locker,
	}
}
