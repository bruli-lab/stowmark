package model_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/stretchr/testify/require"
)

func validConfig() model.Config {
	level := 5
	return model.Config{
		ID:            "7c980d8c-4511-433d-8613-b33196435d60",
		FormatVersion: 2,
		CreatedAt:     "2026-11-13T23:23:14+01:00",
		Compression:   model.Compression{Type: "zstd", Level: &level},
		Encryption: &model.Encryption{
			Type:                 encryption.DefaultEncryptionType,
			KeyEncryption:        encryption.DefaultKeyEncryption,
			EncryptedKey:         "encrypted",
			PublicKeyFingerprint: "fingerprint",
			Generation:           2,
		},
	}
}

func TestBuildConfigDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mutate      func(conf *model.Config)
		wantErr     bool
		expectedErr error
	}{
		{
			name:   "with a valid encrypted config, then it returns the domain config",
			mutate: func(*model.Config) {},
		},
		{
			name:   "with a valid config without encryption, then it returns the domain config",
			mutate: func(conf *model.Config) { conf.Encryption = nil },
		},
		{
			name:    "with an invalid id, then it returns an error",
			mutate:  func(conf *model.Config) { conf.ID = "not-a-uuid" },
			wantErr: true,
		},
		{
			name:        "with an unknown compression type, then it returns an invalid compression type error",
			mutate:      func(conf *model.Config) { conf.Compression.Type = "brotli" },
			expectedErr: repository.ErrInvalidCompressionType,
		},
		{
			name: "with a compression level out of range, then it returns an invalid level error",
			mutate: func(conf *model.Config) {
				level := 99
				conf.Compression.Level = &level
			},
			expectedErr: repository.ErrInvalidZstdLevel,
		},
		{
			name:        "with an unsupported format version, then it returns an unsupported format version error",
			mutate:      func(conf *model.Config) { conf.FormatVersion = 3 },
			expectedErr: repository.ErrUnsupportedFormatVersion,
		},
		{
			name:    "with an invalid created_at, then it returns an error",
			mutate:  func(conf *model.Config) { conf.CreatedAt = "yesterday" },
			wantErr: true,
		},
		{
			name:        "with an unsupported encryption type, then it returns an unsupported encryption error",
			mutate:      func(conf *model.Config) { conf.Encryption.Type = "chacha20-poly1305" },
			expectedErr: encryption.ErrUnsupportedEncryption,
		},
	}

	for _, tt := range tests {
		t.Run(`Given a model config,
		when BuildConfigDomain function is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			conf := validConfig()
			tt.mutate(&conf)

			got, err := model.BuildConfigDomain(conf)

			if tt.wantErr || tt.expectedErr != nil {
				require.Error(t, err)
				if tt.expectedErr != nil {
					require.ErrorIs(t, err, tt.expectedErr)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, conf.ID, got.Id().String())
			require.Equal(t, conf.FormatVersion, got.FormatVersion().Int())
			require.Equal(t, conf.CreatedAt, got.CreatedAt().Format("2006-01-02T15:04:05-07:00"))
			require.Equal(t, conf.Compression.Type, got.Compression().CompType().String())
			if conf.Encryption == nil {
				require.Nil(t, got.Encryption())
				return
			}
			require.Equal(t, conf.Encryption.EncryptedKey, got.Encryption().EncryptedKey())
			require.Equal(t, conf.Encryption.Generation, got.Encryption().Generation())
		})
	}
}
