package encryption

import "errors"

var ErrUnsupportedEncryption = errors.New("unsupported encryption")

const (
	DefaultEncryptionType = "aes-256-gcm"
	DefaultKeyEncryption  = "rsa-oaep-sha256"
)

type EncryptionConfig struct {
	encryptionType       string
	keyEncryption        string
	encryptedKey         string
	publicKeyFingerprint string
	generation           uint64
	encryptedMetadataKey string
}

func (e *EncryptionConfig) EncryptedMetadataKey() string {
	return e.encryptedMetadataKey
}

func (e *EncryptionConfig) HasMetadataKey() bool {
	return e.encryptedMetadataKey != ""
}

func (e *EncryptionConfig) ChangeEncryptedMetadataKey(key string) {
	e.encryptedMetadataKey = key
}

func (e *EncryptionConfig) Generation() uint64 {
	return e.generation
}

func (e *EncryptionConfig) EncryptionType() string {
	return e.encryptionType
}

func (e *EncryptionConfig) KeyEncryption() string {
	return e.keyEncryption
}

func (e *EncryptionConfig) EncryptedKey() string {
	return e.encryptedKey
}

func (e *EncryptionConfig) ChangeEncryptedKey(key string) {
	e.encryptedKey = key
}

func (e *EncryptionConfig) ChangePublicKeyFingerPrint(fingerPrint string) {
	e.publicKeyFingerprint = fingerPrint
}

func (e *EncryptionConfig) PublicKeyFingerprint() string {
	return e.publicKeyFingerprint
}

func (e *EncryptionConfig) ChangeGeneration(i uint64) {
	e.generation = i
}

func NewEncryptionConfig(encryptedKey, publicKeyFingerprint string, generation uint64) *EncryptionConfig {
	return &EncryptionConfig{
		keyEncryption:        DefaultKeyEncryption,
		encryptionType:       DefaultEncryptionType,
		encryptedKey:         encryptedKey,
		publicKeyFingerprint: publicKeyFingerprint,
		generation:           generation,
	}
}

func HydrateEncryptionConfig(
	encryptionType, keyEncryption, encryptedKey, publicKeyFingerprint string,
	generation uint64,
	encryptedMetadataKey string,
) (*EncryptionConfig, error) {
	if encryptionType != DefaultEncryptionType || keyEncryption != DefaultKeyEncryption {
		return nil, ErrUnsupportedEncryption
	}

	conf := NewEncryptionConfig(encryptedKey, publicKeyFingerprint, generation)
	conf.encryptedMetadataKey = encryptedMetadataKey
	return conf, nil
}
