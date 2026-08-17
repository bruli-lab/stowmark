//go:build integration

package stowmark_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

type sshKeyPair struct {
	privateKeyPath string
	publicKey      []byte
}

func createSSHKeyPair(t *testing.T) sshKeyPair {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyDER,
	})

	sshPublicKey, err := ssh.NewPublicKey(publicKey)
	require.NoError(t, err)

	keyPath := filepath.Join(t.TempDir(), "stowmark")
	require.NoError(t, os.WriteFile(keyPath, privateKeyPEM, 0o600))

	return sshKeyPair{
		privateKeyPath: keyPath,
		publicKey:      ssh.MarshalAuthorizedKey(sshPublicKey),
	}
}
