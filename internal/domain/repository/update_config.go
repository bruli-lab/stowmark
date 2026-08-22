package repository

import "github.com/bruli-lab/stowmark/internal/domain/encryption"

func updateConfig(previous, newConfig *Config, force bool, result *InitResult) *Config {
	var encryptionConfig *encryption.EncryptionConfig
	switch {
	case previous.Encryption() != nil && newConfig.encryption == nil && force:
		result.Warnings = append(result.Warnings, "encryption configuration removed.\nExisting encrypted objects may no longer be recoverable.")
		encryptionConfig = newConfig.encryption
	case previous.Encryption() != nil && newConfig.encryption == nil:
		result.Warnings = append(result.Warnings, "the repository already has encryption enabled.\nThe existing encryption configuration has been preserved.")
		encryptionConfig = previous.encryption
	case previous.Encryption() != nil && newConfig.encryption != nil:
		encryptionConfig = previous.encryption
	default:
		encryptionConfig = newConfig.encryption
	}
	return NewConfig(previous.Id(), previous.formatVersion, newConfig.compression, encryptionConfig)
}
