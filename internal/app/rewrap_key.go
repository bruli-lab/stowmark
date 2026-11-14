//nolint:dupl // CQS command handlers intentionally share the same adapter structure.
package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

const RewrapKeyCommandName = "rewrap-key"

type RewrapKeyCommand struct {
	RepositoryPath string
	OldPrivateKey  string
	NewPublicKey   string
}

func (r RewrapKeyCommand) Name() string {
	return RewrapKeyCommandName
}

type RewrapKey struct {
	folderRepo FolderRepository
	keys       keyring
	locker     *RepositoryLocker
}

func (r *RewrapKey) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(RewrapKeyCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(RewrapKeyCommandName, cmd.Name())
	}
	return nil, r.locker.WithLock(ctx, repository.ExclusiveLock, func(ctx context.Context) error {
		return r.rewrap(ctx, co)
	})
}

func (r *RewrapKey) rewrap(ctx context.Context, co RewrapKeyCommand) error {
	conf, err := r.folderRepo.GetConfig(ctx, co.RepositoryPath)
	if err != nil {
		return err
	}
	encryptionConf := conf.Encryption()
	if encryptionConf == nil {
		return ErrRepositoryNotEncrypted
	}
	symmetricKey, err := r.keys.unwrap(ctx, co.OldPrivateKey, encryptionConf)
	if err != nil {
		return err
	}
	defer clear(symmetricKey)
	wrapped, err := r.keys.wrap(ctx, co.NewPublicKey, symmetricKey)
	if err != nil {
		return err
	}
	encryptionConf.ChangeEncryptedKey(wrapped.encodedKey)
	encryptionConf.ChangePublicKeyFingerPrint(wrapped.fingerprint)
	return r.folderRepo.CreateConfig(ctx, co.RepositoryPath, conf)
}

func NewRewrapKey(
	folderRepo FolderRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
) *RewrapKey {
	return &RewrapKey{folderRepo: folderRepo, keys: newKeyring(folderRepo, asymmetricRepo, symmetricRepo), locker: locker}
}
