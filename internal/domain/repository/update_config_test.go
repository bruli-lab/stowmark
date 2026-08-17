package repository

import (
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func Test_updateConfig(t *testing.T) {
	id := uuid.New()
	now := time.Now()
	encryptionConfig := encryption.NewEncryptionConfig("key", "fingerprint", uint64(1))
	type args struct {
		previous  *Config
		newConfig *Config
		force     bool
	}
	tests := []struct {
		name                     string
		args                     args
		expectedEncryptionConfig *encryption.EncryptionConfig
	}{
		{
			name: "with encryption in previous config but no in new config and force is true, it should remove the encryption config",
			args: args{
				previous: &Config{
					id:            id,
					formatVersion: 2,
					createdAt:     now,
					compression: &Compression{
						compType: NoneCompressionType,
					},
					encryption: encryptionConfig,
				},
				newConfig: &Config{
					id:            id,
					formatVersion: 2,
					createdAt:     now,
					compression: &Compression{
						compType: NoneCompressionType,
					},
				},
				force: true,
			},
			expectedEncryptionConfig: nil,
		},
		{
			name: "with encryption in previous config but no in new config and no force, it should maintain the encryption config",
			args: args{
				previous: &Config{
					id:            id,
					formatVersion: 2,
					createdAt:     now,
					compression: &Compression{
						compType: NoneCompressionType,
					},
					encryption: encryptionConfig,
				},
				newConfig: &Config{
					id:            id,
					formatVersion: 2,
					createdAt:     now,
					compression: &Compression{
						compType: NoneCompressionType,
					},
				},
				force: false,
			},
			expectedEncryptionConfig: encryptionConfig,
		},
		{
			name: "with encryption in previous config and encryption in new config, it should maintain the encryption config",
			args: args{
				previous: &Config{
					id:            id,
					formatVersion: 2,
					createdAt:     now,
					compression: &Compression{
						compType: NoneCompressionType,
					},
					encryption: encryptionConfig,
				},
				newConfig: &Config{
					id:            id,
					formatVersion: 2,
					createdAt:     now,
					compression: &Compression{
						compType: NoneCompressionType,
					},
					encryption: encryption.NewEncryptionConfig("key2", "fingerprint2", uint64(1)),
				},
				force: false,
			},
			expectedEncryptionConfig: encryptionConfig,
		},
		{
			name: "without encryption in previous config and encryption in new config, it should update the encryption config",
			args: args{
				previous: &Config{
					id:            id,
					formatVersion: 2,
					createdAt:     now,
					compression: &Compression{
						compType: NoneCompressionType,
					},
				},
				newConfig: &Config{
					id:            id,
					formatVersion: 2,
					createdAt:     now,
					compression: &Compression{
						compType: NoneCompressionType,
					},
					encryption: encryptionConfig,
				},
				force: false,
			},
			expectedEncryptionConfig: encryptionConfig,
		},
	}
	for _, tt := range tests {
		t.Run(`Given an updateConfig function,
		whe is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			got := updateConfig(tt.args.previous, tt.args.newConfig, tt.args.force, &InitResult{})
			require.Equal(t, tt.expectedEncryptionConfig, got.Encryption())
		})
	}
}
