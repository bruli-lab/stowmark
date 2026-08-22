package encryption

import (
	"context"
	"sync"
)

type DecryptSymmetricKey struct {
	symmetricKeyRepository SymmetricKeyRepository
	asymmetricRepo         AsymmetricKeyPairRepository
	mu                     sync.Mutex
	symmetricKey           []byte
}

func (d *DecryptSymmetricKey) Decrypt(ctx context.Context, privateKeyPath, encodedKey string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.symmetricKey) == 0 {
		privateKey, err := d.asymmetricRepo.ReadRSAPrivateKey(ctx, privateKeyPath)
		if err != nil {
			return nil, err
		}
		symmetricKey, err := d.symmetricKeyRepository.DecodeAndDecryptSymmetricKey(ctx, privateKey, encodedKey)
		if err != nil {
			return nil, err
		}
		d.symmetricKey = symmetricKey
	}
	return d.symmetricKey, nil
}

func NewDecryptSymmetricKey(symmetricKeyRepository SymmetricKeyRepository, asymmetricRepo AsymmetricKeyPairRepository) *DecryptSymmetricKey {
	return &DecryptSymmetricKey{symmetricKeyRepository: symmetricKeyRepository, asymmetricRepo: asymmetricRepo}
}
