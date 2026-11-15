package disk_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/stretchr/testify/require"
)

func TestAsymmetricKeyPairRepository_CreateKeys(t *testing.T) {
	t.Parallel()

	privateKey, err := rsa.GenerateKey(rand.Reader, disk.MinimumRSAKeySizeInBit)
	require.NoError(t, err)

	t.Run(`Given an AsymmetricKeyPairRepository,
	when CreatePrivateKey and CreatePublicKey methods are called,
	then the private key is only readable by its owner and both keys can be read back`, func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("POSIX permissions are not available on Windows")
		}

		ctx := context.Background()
		folder := t.TempDir()
		privatePath := filepath.Join(folder, "keys", "private.pem")
		publicPath := filepath.Join(folder, "keys", "public.pem")
		repo := disk.NewAsymmetricKeyPairRepository()

		require.NoError(t, repo.CreatePrivateKey(ctx, privatePath, privateKey))
		require.NoError(t, repo.CreatePublicKey(ctx, publicPath, &privateKey.PublicKey))

		privateInfo, err := os.Stat(privatePath)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), privateInfo.Mode().Perm())

		publicInfo, err := os.Stat(publicPath)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o644), publicInfo.Mode().Perm())

		readPrivate, err := repo.ReadRSAPrivateKey(ctx, privatePath)
		require.NoError(t, err)
		require.True(t, privateKey.Equal(readPrivate))

		readPublic, err := repo.ReadRSAPublicKey(ctx, publicPath)
		require.NoError(t, err)
		require.True(t, privateKey.PublicKey.Equal(readPublic))
	})

	t.Run(`Given an AsymmetricKeyPairRepository and an existing key file,
	when CreatePrivateKey method is called with the same path,
	then it returns an error and keeps the existing file`, func(t *testing.T) {
		t.Parallel()

		privatePath := filepath.Join(t.TempDir(), "private.pem")
		existing := []byte("existing key")
		require.NoError(t, os.WriteFile(privatePath, existing, 0o600))

		err := disk.NewAsymmetricKeyPairRepository().CreatePrivateKey(context.Background(), privatePath, privateKey)

		require.ErrorIs(t, err, os.ErrExist)
		content, readErr := os.ReadFile(privatePath)
		require.NoError(t, readErr)
		require.Equal(t, existing, content)
	})
}
