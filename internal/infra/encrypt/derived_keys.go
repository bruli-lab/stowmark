package encrypt

import (
	"crypto/hkdf"
	"crypto/sha256"
	"fmt"
)

const (
	objectIDKeyInfo = "stowmark object id v1"
	manifestKeyInfo = "stowmark manifest v1"
)

func ObjectIDKey(metadataKey []byte) ([]byte, error) {
	return deriveKey(metadataKey, objectIDKeyInfo)
}

func ManifestKey(metadataKey []byte) ([]byte, error) {
	return deriveKey(metadataKey, manifestKeyInfo)
}

func deriveKey(metadataKey []byte, info string) ([]byte, error) {
	if len(metadataKey) != AES256KeySize {
		return nil, fmt.Errorf("%w: got %d bytes, expected %d", ErrInvalidKeySize, len(metadataKey), AES256KeySize)
	}
	key, err := hkdf.Key(sha256.New, metadataKey, nil, info, AES256KeySize)
	if err != nil {
		return nil, fmt.Errorf("derive key %q: %w", info, err)
	}
	return key, nil
}
