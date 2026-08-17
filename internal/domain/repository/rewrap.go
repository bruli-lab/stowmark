package repository

import (
	"context"
	"encoding/base64"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
)

type Rewrap struct {
	folderRepository  FolderRepository
	symmetricKeyRepo  encryption.SymmetricKeyRepository
	asymmetricKeyRepo encryption.AsymmetricKeyPairRepository
}

func (r Rewrap) Do(ctx context.Context, repositoryPath, oldPrivateKeyPath, newPublicKeyPath string) error {
	conf, err := r.folderRepository.GetConfig(ctx, repositoryPath)
	if err != nil {
		return err
	}
	privateKey, err := r.asymmetricKeyRepo.ReadRSAPrivateKey(ctx, oldPrivateKeyPath)
	if err != nil {
		return err
	}
	symmetricKey, err := r.symmetricKeyRepo.DecodeAndDecryptSymmetricKey(ctx, privateKey, conf.Encryption().EncryptedKey())
	if err != nil {
		return err
	}
	publicKey, err := r.asymmetricKeyRepo.ReadRSAPublicKey(ctx, newPublicKeyPath)
	if err != nil {
		return err
	}
	fingerPrint, err := r.asymmetricKeyRepo.PublicKeyFingerPrint(ctx, publicKey)
	if err != nil {
		return err
	}
	encryptedKey, err := r.symmetricKeyRepo.EncryptSymmetricKey(ctx, publicKey, symmetricKey)
	if err != nil {
		return err
	}
	conf.Encryption().ChangeEncryptedKey(base64.StdEncoding.EncodeToString(encryptedKey))
	conf.Encryption().ChangePublicKeyFingerPrint(fingerPrint)
	return r.folderRepository.CreateConfig(ctx, repositoryPath, conf)
}

func NewRewrap(
	folderRepository FolderRepository,
	symmetricKeyRepo encryption.SymmetricKeyRepository,
	asymmetricKeyRepo encryption.AsymmetricKeyPairRepository,
) *Rewrap {
	return &Rewrap{folderRepository: folderRepository, symmetricKeyRepo: symmetricKeyRepo, asymmetricKeyRepo: asymmetricKeyRepo}
}
