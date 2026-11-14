package repository

import (
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestConfig_Reinitialize(t *testing.T) {
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
		t.Run(`Given a previous config,
		when it is reinitialized `+tt.name, func(t *testing.T) {
			t.Parallel()
			got, _ := tt.args.previous.Reinitialize(tt.args.newConfig, tt.args.force)
			require.Equal(t, tt.expectedEncryptionConfig, got.Encryption())
		})
	}
}

func TestConfig_ReinitializeKeepsCreatedAt(t *testing.T) {
	t.Parallel()

	t.Run(`Given a previous config created in the past,
	when it is reinitialized with a new config,
	then it keeps the creation date of the previous config`, func(t *testing.T) {
		t.Parallel()

		createdAt := time.Date(2025, time.March, 1, 10, 0, 0, 0, time.UTC)
		previous := HydrateConfig(uuid.New(), FormatVersionTwo, createdAt, &Compression{compType: NoneCompressionType}, nil)
		newConfig := NewConfig(uuid.New(), FormatVersionTwo, &Compression{compType: ZstdCompressionType}, nil)

		got, warnings := previous.Reinitialize(newConfig, false)

		require.Equal(t, createdAt, got.CreatedAt())
		require.Equal(t, previous.ID(), got.ID())
		require.Equal(t, ZstdCompressionType, got.Compression().CompType())
		require.Empty(t, warnings)
	})
}
