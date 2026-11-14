package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
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
	folderRepo FolderRepository
	objectRepo ObjectRepository
	keys       keyring
}

func (r *RekeyKey) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(RekeyKeyCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(RekeyKeyCommandName, cmd.Name())
	}
	return nil, r.rekey(ctx, co)
}

func (r *RekeyKey) rekey(ctx context.Context, co RekeyKeyCommand) (resultErr error) {
	conf, err := r.folderRepo.GetConfig(ctx, co.RepositoryPath)
	if err != nil {
		return err
	}

	encryptionConf := conf.Encryption()
	if encryptionConf == nil {
		return ErrMissingEncryptionConfigToEncrypt
	}

	currentGeneration := encryptionConf.Generation()
	newGeneration := currentGeneration + 1

	oldSymmetricKey, err := r.keys.unwrap(ctx, co.PrivateKeyPath, encryptionConf)
	if err != nil {
		return err
	}

	newSymmetricKey, err := r.keys.symmetricRepo.GenerateSymmetricKey(ctx)
	if err != nil {
		return err
	}

	wrapped, err := r.keys.wrap(ctx, co.PublicKeyPath, newSymmetricKey)
	if err != nil {
		return err
	}

	hashes, err := r.objectRepo.ListEncryptedObjects(ctx, currentGeneration)
	if err != nil {
		return err
	}

	committed := false

	defer func() {
		if committed {
			return
		}

		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rekeyCleanupTimeout)
		defer cancel()

		if cleanupErr := r.objectRepo.AbortRekey(cleanupCtx, newGeneration); cleanupErr != nil {
			resultErr = errors.Join(resultErr, cleanupErr)
		}
	}()

	if err := r.rekeyObjects(ctx, hashes, currentGeneration, newGeneration, oldSymmetricKey, newSymmetricKey); err != nil {
		return err
	}

	encryptionConf.ChangeGeneration(newGeneration)
	encryptionConf.ChangeEncryptedKey(wrapped.encodedKey)
	encryptionConf.ChangePublicKeyFingerPrint(wrapped.fingerprint)

	if err := r.folderRepo.CreateConfig(ctx, co.RepositoryPath, conf); err != nil {
		return err
	}

	committed = true

	return r.objectRepo.DeleteEncryptedGeneration(ctx, currentGeneration)
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
	reader, err := r.objectRepo.ReadEncryptedObject(ctx, hash, currentGeneration, oldSymmetricKey)
	if err != nil {
		return err
	}

	saveErr := r.objectRepo.SaveRekeyedObject(ctx, hash, reader, newGeneration, newSymmetricKey)
	closeErr := reader.Close()

	return errors.Join(saveErr, closeErr)
}

func NewRekeyKey(
	folderRepo FolderRepository,
	objectRepo ObjectRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
) *RekeyKey {
	return &RekeyKey{
		folderRepo: folderRepo,
		objectRepo: objectRepo,
		keys:       newKeyring(folderRepo, asymmetricRepo, symmetricRepo),
	}
}
