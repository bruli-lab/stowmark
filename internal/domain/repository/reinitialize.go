package repository

import "github.com/bruli-lab/stowmark/internal/domain/encryption"

const (
	EncryptionRemovedWarning   = "encryption configuration removed.\nExisting encrypted objects may no longer be recoverable."
	EncryptionPreservedWarning = "the repository already has encryption enabled.\nThe existing encryption configuration has been preserved."
)

func (c Config) Reinitialize(newConfig *Config, force bool) (reinitialized *Config, warnings []string) {
	var encryptionConfig *encryption.EncryptionConfig
	switch {
	case c.encryption != nil && newConfig.encryption == nil && force:
		warnings = append(warnings, EncryptionRemovedWarning)
		encryptionConfig = newConfig.encryption
	case c.encryption != nil && newConfig.encryption == nil:
		warnings = append(warnings, EncryptionPreservedWarning)
		encryptionConfig = c.encryption
	case c.encryption != nil:
		encryptionConfig = c.encryption
	default:
		encryptionConfig = newConfig.encryption
	}
	return HydrateConfig(c.id, c.formatVersion, c.createdAt, newConfig.compression, encryptionConfig), warnings
}
