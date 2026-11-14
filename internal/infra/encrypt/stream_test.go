package encrypt_test

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/stretchr/testify/require"
)

func encryptStream(t *testing.T, key, aad, plaintext []byte) []byte {
	t.Helper()
	var encrypted bytes.Buffer
	writer, err := encrypt.NewStreamWriter(&encrypted, key, aad)
	require.NoError(t, err)
	_, err = writer.Write(plaintext)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return encrypted.Bytes()
}

func decryptStream(key, aad, encrypted []byte) ([]byte, error) {
	reader, err := encrypt.NewStreamReader(bytes.NewReader(encrypted), key, aad)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(reader)
}

func TestStream_RoundTrip(t *testing.T) {
	t.Parallel()

	key := bytes.Repeat([]byte{3}, encrypt.AES256KeySize)

	tests := []struct {
		name string
		size int
	}{
		{name: "with empty data", size: 0},
		{name: "with less than one segment", size: 100},
		{name: "with exactly one segment", size: encrypt.StreamSegmentSize},
		{name: "with exactly two segments", size: 2 * encrypt.StreamSegmentSize},
		{name: "with several segments and a partial one", size: 3*encrypt.StreamSegmentSize + 17},
	}

	for _, tt := range tests {
		t.Run(`Given an encryption stream,
		when data is encrypted and decrypted `+tt.name+`,
		then it returns the same data and the ciphertext is not the plaintext`, func(t *testing.T) {
			t.Parallel()

			plaintext := make([]byte, tt.size)
			_, _ = rand.Read(plaintext)

			encrypted := encryptStream(t, key, []byte("object-id"), plaintext)
			decrypted, err := decryptStream(key, []byte("object-id"), encrypted)

			require.NoError(t, err)
			require.Equal(t, plaintext, append([]byte{}, decrypted...))
			require.True(t, encrypt.IsStream(encrypted))
			if tt.size > 0 {
				require.NotContains(t, string(encrypted), string(plaintext))
			}
		})
	}

	t.Run(`Given the same data encrypted twice,
	when the ciphertexts are compared,
	then they differ because every stream has its own salt`, func(t *testing.T) {
		t.Parallel()

		plaintext := []byte("same data")

		first := encryptStream(t, key, nil, plaintext)
		second := encryptStream(t, key, nil, plaintext)

		require.NotEqual(t, first, second)
	})
}

func TestStream_Tampering(t *testing.T) {
	t.Parallel()

	key := bytes.Repeat([]byte{5}, encrypt.AES256KeySize)
	plaintext := bytes.Repeat([]byte("stowmark"), encrypt.StreamSegmentSize/4)
	encrypted := encryptStream(t, key, []byte("object-a"), plaintext)
	segment := encrypt.StreamSegmentSize + 16
	header := len(encrypted) - 2*segment

	require.Equal(t, 36, header)

	tests := []struct {
		name      string
		key       []byte
		aad       []byte
		encrypted func() []byte
	}{
		{
			name:      "with another associated data, then it fails because the stream belongs to another object",
			key:       key,
			aad:       []byte("object-b"),
			encrypted: func() []byte { return encrypted },
		},
		{
			name:      "with another key, then it fails",
			key:       bytes.Repeat([]byte{6}, encrypt.AES256KeySize),
			aad:       []byte("object-a"),
			encrypted: func() []byte { return encrypted },
		},
		{
			name: "with a flipped byte, then it fails",
			key:  key,
			aad:  []byte("object-a"),
			encrypted: func() []byte {
				changed := bytes.Clone(encrypted)
				changed[len(changed)/2] ^= 1
				return changed
			},
		},
		{
			name:      "with the last segment removed, then it fails because the stream is truncated",
			key:       key,
			aad:       []byte("object-a"),
			encrypted: func() []byte { return encrypted[:header+segment] },
		},
		{
			name:      "with only the header, then it fails because the stream is truncated",
			key:       key,
			aad:       []byte("object-a"),
			encrypted: func() []byte { return encrypted[:header] },
		},
		{
			name: "with data appended after the last segment, then it fails",
			key:  key,
			aad:  []byte("object-a"),
			encrypted: func() []byte {
				return append(bytes.Clone(encrypted), make([]byte, 32)...)
			},
		},
		{
			name:      "with data that is not a stream, then it fails",
			key:       key,
			aad:       []byte("object-a"),
			encrypted: func() []byte { return bytes.Repeat([]byte("x"), 64) },
		},
	}

	for _, tt := range tests {
		t.Run(`Given an encrypted stream,
		when it is decrypted `+tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := decryptStream(tt.key, tt.aad, tt.encrypted())

			require.ErrorIs(t, err, encrypt.ErrDecryptionFailed)
		})
	}
}

func TestDerivedKeys(t *testing.T) {
	t.Parallel()

	t.Run(`Given a metadata key,
	when the object id and manifest keys are derived,
	then they are stable, different from each other and from the metadata key`, func(t *testing.T) {
		t.Parallel()

		metadataKey := bytes.Repeat([]byte{9}, encrypt.AES256KeySize)

		objectID, err := encrypt.ObjectIDKey(metadataKey)
		require.NoError(t, err)
		again, err := encrypt.ObjectIDKey(metadataKey)
		require.NoError(t, err)
		manifest, err := encrypt.ManifestKey(metadataKey)
		require.NoError(t, err)

		require.Equal(t, objectID, again)
		require.NotEqual(t, objectID, manifest)
		require.NotEqual(t, metadataKey, objectID)
		require.Len(t, manifest, encrypt.AES256KeySize)
	})

	t.Run(`Given a metadata key with a wrong size,
	when a key is derived,
	then it returns an invalid key size error`, func(t *testing.T) {
		t.Parallel()

		_, err := encrypt.ManifestKey([]byte("short"))

		require.ErrorIs(t, err, encrypt.ErrInvalidKeySize)
	})
}
