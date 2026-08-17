package snapshot

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

type EncryptionData struct {
	Generation   uint64
	SymmetricKey []byte
}

type SymmetricKeyHandler struct {
	folderRepo    repository.FolderRepository
	decryptKeySvc *encryption.DecryptSymmetricKey
}

func (s SymmetricKeyHandler) Handle(ctx context.Context, privateKeyPath *string, repositoryPath string) (*EncryptionData, error) {
	if privateKeyPath != nil {
		conf, err := s.folderRepo.GetConfig(ctx, repositoryPath)
		if err != nil {
			return nil, err
		}
		if conf.Encryption() == nil {
			return nil, ErrMissingEncryptionConfigToEncrypt
		}
		symmetricKey, err := s.decryptKeySvc.Decrypt(ctx, *privateKeyPath, conf.Encryption().EncryptedKey())
		if err != nil {
			return nil, err
		}
		return &EncryptionData{
			Generation:   conf.Encryption().Generation(),
			SymmetricKey: symmetricKey,
		}, nil
	}
	return nil, nil
}

func newSymmetricKeyHandler(folderRepo repository.FolderRepository, decryptKeySvc *encryption.DecryptSymmetricKey) *SymmetricKeyHandler {
	return &SymmetricKeyHandler{folderRepo: folderRepo, decryptKeySvc: decryptKeySvc}
}
