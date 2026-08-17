package snapshot

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"golang.org/x/sync/errgroup"
)

const maxRekeyWorkers = 8

type ReKey struct {
	folderRepo        repository.FolderRepository
	symmetricKeyRepo  encryption.SymmetricKeyRepository
	asymmetricKeyRepo encryption.AsymmetricKeyPairRepository
	objectRepo        ObjectRepository
}

func (r *ReKey) Do(ctx context.Context, repositoryPath, privateKeyPath, publicKeyPath string) (resultErr error) {
	conf, err := r.folderRepo.GetConfig(ctx, repositoryPath)
	if err != nil {
		return err
	}

	encryptionConf := conf.Encryption()
	if encryptionConf == nil {
		return ErrMissingEncryptionConfigToEncrypt
	}

	currentGeneration := encryptionConf.Generation()
	newGeneration := currentGeneration + 1

	privateKey, err := r.asymmetricKeyRepo.ReadRSAPrivateKey(ctx, privateKeyPath)
	if err != nil {
		return err
	}

	oldSymmetricKey, err := r.symmetricKeyRepo.DecodeAndDecryptSymmetricKey(
		ctx,
		privateKey,
		encryptionConf.EncryptedKey(),
	)
	if err != nil {
		return err
	}

	publicKey, err := r.asymmetricKeyRepo.ReadRSAPublicKey(ctx, publicKeyPath)
	if err != nil {
		return err
	}

	newSymmetricKey, err := r.symmetricKeyRepo.GenerateSymmetricKey(ctx)
	if err != nil {
		return err
	}

	encryptedNewKey, err := r.symmetricKeyRepo.EncryptSymmetricKey(ctx, publicKey, newSymmetricKey)
	if err != nil {
		return err
	}

	fingerprint, err := r.asymmetricKeyRepo.PublicKeyFingerPrint(ctx, publicKey)
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

		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cleanupErr := r.objectRepo.AbortRekey(cleanupCtx, newGeneration)
		if cleanupErr != nil {
			resultErr = errors.Join(resultErr, cleanupErr)
		}
	}()

	if err := r.rekeyObjects(ctx, hashes, currentGeneration, newGeneration, oldSymmetricKey, newSymmetricKey); err != nil {
		return err
	}

	encryptionConf.ChangeGeneration(newGeneration)
	encryptionConf.ChangeEncryptedKey(base64.StdEncoding.EncodeToString(encryptedNewKey))
	encryptionConf.ChangePublicKeyFingerPrint(fingerprint)

	if err := r.folderRepo.CreateConfig(ctx, repositoryPath, conf); err != nil {
		return err
	}

	committed = true

	if err := r.objectRepo.DeleteEncryptedGeneration(ctx, currentGeneration); err != nil {
		return err
	}

	return nil
}

func (r *ReKey) rekeyObjects(ctx context.Context, hashes []string, currentGeneration, newGeneration uint64, oldSymmetricKey, newSymmetricKey []byte) error {
	if len(hashes) == 0 {
		return nil
	}

	workers := min(maxRekeyWorkers, len(hashes))
	group, groupCtx := errgroup.WithContext(ctx)
	jobs := make(chan string)

	group.Go(func() error {
		defer close(jobs)

		for _, hash := range hashes {
			select {
			case jobs <- hash:
			case <-groupCtx.Done():
				return nil
			}
		}

		return nil
	})

	for range workers {
		group.Go(func() error {
			for {
				select {
				case <-groupCtx.Done():
					return nil

				case hash, ok := <-jobs:
					if !ok {
						return nil
					}

					if err := r.rekeyObject(
						groupCtx,
						hash,
						currentGeneration,
						newGeneration,
						oldSymmetricKey,
						newSymmetricKey,
					); err != nil {
						return fmt.Errorf("rekey object %q: %w", hash, err)
					}
				}
			}
		})
	}

	return group.Wait()
}

func (r *ReKey) rekeyObject(
	ctx context.Context,
	hash string,
	currentGeneration, newGeneration uint64,
	oldSymmetricKey, newSymmetricKey []byte,
) error {
	reader, err := r.objectRepo.ReadEncryptedObject(
		ctx,
		hash,
		currentGeneration,
		oldSymmetricKey,
	)
	if err != nil {
		return err
	}

	saveErr := r.objectRepo.SaveRekeyedObject(
		ctx,
		hash,
		reader,
		newGeneration,
		newSymmetricKey,
	)

	closeErr := reader.Close()

	return errors.Join(saveErr, closeErr)
}

func NewReKey(
	folderRepo repository.FolderRepository,
	symmetricKeyRepo encryption.SymmetricKeyRepository,
	asymmetricKeyRepo encryption.AsymmetricKeyPairRepository,
	objectRepo ObjectRepository,
) *ReKey {
	return &ReKey{
		folderRepo:        folderRepo,
		symmetricKeyRepo:  symmetricKeyRepo,
		asymmetricKeyRepo: asymmetricKeyRepo,
		objectRepo:        objectRepo,
	}
}
