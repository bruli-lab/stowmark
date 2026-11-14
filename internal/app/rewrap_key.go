//nolint:dupl // CQS command handlers intentionally share the same adapter structure.
package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
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
	return withLockedEvent(ctx, r.locker, repository.ExclusiveLock, func(ctx context.Context) (*encryption.KeyRewrapped, error) {
		return r.rewrap(ctx, co)
	})
}

func (r *RewrapKey) rewrap(ctx context.Context, co RewrapKeyCommand) (*encryption.KeyRewrapped, error) {
	conf, err := r.folderRepo.GetConfig(ctx, co.RepositoryPath)
	if err != nil {
		return nil, err
	}
	encryptionConf := conf.Encryption()
	if encryptionConf == nil {
		return nil, ErrRepositoryNotEncrypted
	}
	symmetricKey, err := r.keys.unwrap(ctx, co.OldPrivateKey, encryptionConf)
	if err != nil {
		return nil, err
	}
	defer clear(symmetricKey)
	wrapped, err := r.keys.wrap(ctx, co.NewPublicKey, symmetricKey)
	if err != nil {
		return nil, err
	}
	encryptionConf.ChangeEncryptedKey(wrapped.encodedKey)
	encryptionConf.ChangePublicKeyFingerPrint(wrapped.fingerprint)
	if err := r.folderRepo.CreateConfig(ctx, co.RepositoryPath, conf); err != nil {
		return nil, err
	}
	return encryptionConf.KeyRewrapped(conf.ID().String()), nil
}

func NewRewrapKey(
	folderRepo FolderRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
) *RewrapKey {
	return &RewrapKey{folderRepo: folderRepo, keys: newKeyring(folderRepo, asymmetricRepo, symmetricRepo), locker: locker}
}
