package model

import (
	"fmt"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/google/uuid"
)

const ConfigFile = "config.json"

type Compression struct {
	Type  string `json:"type"`
	Level *int   `json:"level,omitempty"`
}

type Encryption struct {
	Type                 string `json:"type"`
	KeyEncryption        string `json:"key_encryption"`
	EncryptedKey         string `json:"encrypted_key"`
	PublicKeyFingerprint string `json:"public_key_fingerprint"`
	Generation           uint64 `json:"generation"`
}
type Config struct {
	ID            string      `json:"id"`
	FormatVersion int         `json:"format_version"`
	CreatedAt     string      `json:"created_at"`
	Compression   Compression `json:"compression"`
	Encryption    *Encryption `json:"encryption,omitempty"`
}

func BuildConfigDomain(conf Config) (*repository.Config, error) {
	id, err := uuid.Parse(conf.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config id: %w", err)
	}
	compType, err := repository.ParseCompressionType(conf.Compression.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to parse compression type: %w", err)
	}
	comp, err := repository.NewCompression(*compType, conf.Compression.Level)
	if err != nil {
		return nil, fmt.Errorf("failed to create compression: %w", err)
	}
	var encryptionConfig *encryption.EncryptionConfig
	if conf.Encryption != nil {
		encryptionConfig, err = encryption.HydrateEncryptionConfig(
			conf.Encryption.Type,
			conf.Encryption.KeyEncryption,
			conf.Encryption.EncryptedKey,
			conf.Encryption.PublicKeyFingerprint,
			conf.Encryption.Generation,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to parse config encryption %q/%q: %w", conf.Encryption.Type, conf.Encryption.KeyEncryption, err)
		}
	}
	formatVersion, err := repository.ParseFormatVersion(conf.FormatVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config format version: %w", err)
	}
	createdAt, err := time.Parse(time.RFC3339, conf.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config created_at: %w", err)
	}
	return repository.HydrateConfig(id, formatVersion, createdAt, comp, encryptionConfig), nil
}
