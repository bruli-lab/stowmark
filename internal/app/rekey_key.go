//nolint:dupl // CQS command handlers intentionally share the same adapter structure.
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"golang.org/x/sync/errgroup"
)

const (
	RekeyKeyCommandName = "rekey-key"
	maxRekeyWorkers     = 8
	rekeyCleanupTimeout = 30 * time.Second
)

type RekeyKeyCommand struct {
	RepositoryPath string
	PrivateKeyPath string
	PublicKeyPath  string
}

func (r RekeyKeyCommand) Name() string {
	return RekeyKeyCommandName
}

type RekeyKey struct {
	folderRepo    FolderRepository
	encryptedRepo EncryptedObjectRepository
	keys          keyring
	locker        *RepositoryLocker
}

func (r *RekeyKey) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(RekeyKeyCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(RekeyKeyCommandName, cmd.Name())
	}
	return withLockedEvent(ctx, r.locker, repository.ExclusiveLock, func(ctx context.Context) (*encryption.SymmetricKeyRotated, error) {
		return r.rekey(ctx, co)
	})
}

func (r *RekeyKey) rekey(ctx context.Context, co RekeyKeyCommand) (rotated *encryption.SymmetricKeyRotated, resultErr error) {
	conf, err := r.folderRepo.GetConfig(ctx, co.RepositoryPath)
	if err != nil {
		return nil, err
	}

	encryptionConf := conf.Encryption()
	if encryptionConf == nil {
		return nil, ErrMissingEncryptionConfigToEncrypt
	}

	currentGeneration := encryptionConf.Generation()
	newGeneration := currentGeneration + 1

	oldSymmetricKey, err := r.keys.unwrap(ctx, co.PrivateKeyPath, encryptionConf)
	if err != nil {
		return nil, err
	}

	newSymmetricKey, err := r.keys.symmetricRepo.GenerateSymmetricKey(ctx)
	if err != nil {
		return nil, err
	}

	wrapped, err := r.keys.wrap(ctx, co.PublicKeyPath, newSymmetricKey)
	if err != nil {
		return nil, err
	}

	hashes, err := r.encryptedRepo.ListEncryptedObjects(ctx, currentGeneration)
	if err != nil {
		return nil, err
	}

	committed := false

	defer func() {
		if committed {
			return
		}

		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rekeyCleanupTimeout)
		defer cancel()

		if cleanupErr := r.encryptedRepo.AbortRekey(cleanupCtx, newGeneration); cleanupErr != nil {
			resultErr = errors.Join(resultErr, cleanupErr)
		}
	}()

	if err := r.rekeyObjects(ctx, hashes, currentGeneration, newGeneration, oldSymmetricKey, newSymmetricKey); err != nil {
		return nil, err
	}

	encryptionConf.ChangeGeneration(newGeneration)
	encryptionConf.ChangeEncryptedKey(wrapped.encodedKey)
	encryptionConf.ChangePublicKeyFingerPrint(wrapped.fingerprint)

	if err := r.folderRepo.CreateConfig(ctx, co.RepositoryPath, conf); err != nil {
		return nil, err
	}

	committed = true

	if err := r.encryptedRepo.DeleteEncryptedGeneration(ctx, currentGeneration); err != nil {
		return nil, err
	}

	return encryptionConf.SymmetricKeyRotated(conf.ID().String(), len(hashes)), nil
}

func (r *RekeyKey) rekeyObjects(ctx context.Context, hashes []string, currentGeneration, newGeneration uint64, oldSymmetricKey, newSymmetricKey []byte) error {
	if len(hashes) == 0 {
		return nil
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(min(maxRekeyWorkers, len(hashes)))

	for _, hash := range hashes {
		if groupCtx.Err() != nil {
			break
		}

		group.Go(func() error {
			if err := r.rekeyObject(groupCtx, hash, currentGeneration, newGeneration, oldSymmetricKey, newSymmetricKey); err != nil {
				return fmt.Errorf("rekey object %q: %w", hash, err)
			}
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return err
	}

	return ctx.Err()
}

func (r *RekeyKey) rekeyObject(
	ctx context.Context,
	hash string,
	currentGeneration, newGeneration uint64,
	oldSymmetricKey, newSymmetricKey []byte,
) error {
	reader, err := r.encryptedRepo.ReadEncryptedObject(ctx, hash, currentGeneration, oldSymmetricKey)
	if err != nil {
		return err
	}

	saveErr := r.encryptedRepo.SaveRekeyedObject(ctx, hash, reader, newGeneration, newSymmetricKey)
	closeErr := reader.Close()

	return errors.Join(saveErr, closeErr)
}

func NewRekeyKey(
	folderRepo FolderRepository,
	encryptedRepo EncryptedObjectRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
) *RekeyKey {
	return &RekeyKey{
		folderRepo:    folderRepo,
		encryptedRepo: encryptedRepo,
		keys:          newKeyring(folderRepo, asymmetricRepo, symmetricRepo),
		locker:        locker,
	}
}
