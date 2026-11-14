package encrypt_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"testing"

	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/stretchr/testify/require"
)

var errWriteFailed = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWriteFailed
}

type stalledWriter struct{}

func (stalledWriter) Write([]byte) (int, error) {
	return 0, nil
}

func newKey(t *testing.T) []byte {
	t.Helper()

	key := make([]byte, encrypt.AES256KeySize)
	_, err := rand.Read(key)
	require.NoError(t, err)

	return key
}

func seal(t *testing.T, key, data []byte) []byte {
	t.Helper()

	var buffer bytes.Buffer
	encoder, err := encrypt.NewAESGCMHandler().Encode(&buffer, key)
	require.NoError(t, err)
	_, err = encoder.Writer.Write(data)
	require.NoError(t, err)
	require.NoError(t, encoder.Closer())

	return buffer.Bytes()
}

func TestAESGCMHandler_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run(`Given an AESGCMHandler and a key,
	when the same data is encrypted twice and decrypted,
	then each ciphertext is different and both decrypt to the original data`, func(t *testing.T) {
		t.Parallel()

		key := newKey(t)
		data := bytes.Repeat([]byte("secret "), 1024)

		first := seal(t, key, data)
		second := seal(t, key, data)

		require.NotEqual(t, first, second)
		for _, sealed := range [][]byte{first, second} {
			reader, err := encrypt.NewAESGCMHandler().Decode(bytes.NewReader(sealed), key)
			require.NoError(t, err)
			plaintext, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, data, plaintext)
		}
	})

	t.Run(`Given an AESGCMHandler encoder,
	when Closer is called twice,
	then the data is written only once`, func(t *testing.T) {
		t.Parallel()

		var buffer bytes.Buffer
		encoder, err := encrypt.NewAESGCMHandler().Encode(&buffer, newKey(t))
		require.NoError(t, err)
		_, err = encoder.Writer.Write([]byte("data"))
		require.NoError(t, err)

		require.NoError(t, encoder.Closer())
		size := buffer.Len()
		require.NoError(t, encoder.Closer())

		require.Equal(t, size, buffer.Len())
	})
}

func TestAESGCMHandler_Decode_Errors(t *testing.T) {
	t.Parallel()

	key := newKey(t)
	sealed := seal(t, key, []byte("authenticated data"))

	tests := []struct {
		name   string
		key    []byte
		source func() []byte
	}{
		{
			name:   "with another key, then it returns an error",
			key:    newKey(t),
			source: func() []byte { return sealed },
		},
		{
			name: "with a tampered ciphertext, then it returns an error",
			key:  key,
			source: func() []byte {
				tampered := bytes.Clone(sealed)
				tampered[len(tampered)-1] ^= 0x01
				return tampered
			},
		},
		{
			name:   "with data shorter than the nonce, then it returns an error",
			key:    key,
			source: func() []byte { return sealed[:4] },
		},
		{
			name:   "with a key of an invalid size, then it returns an error",
			key:    key[:7],
			source: func() []byte { return sealed },
		},
	}

	for _, tt := range tests {
		t.Run(`Given an AESGCMHandler,
		when Decode method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := encrypt.NewAESGCMHandler().Decode(bytes.NewReader(tt.source()), tt.key)

			require.Error(t, err)
		})
	}
}

func TestAESGCMHandler_Encode_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		destination io.Writer
		key         []byte
		expectedErr error
		closeErr    error
	}{
		{
			name:        "without a destination, then it returns a destination required error",
			key:         newKey(t),
			expectedErr: encrypt.ErrDestinationRequired,
		},
		{
			name:        "with a key of an invalid size, then it returns an invalid key size error",
			destination: io.Discard,
			key:         []byte("short"),
			expectedErr: encrypt.ErrInvalidKeySize,
		},
		{
			name:        "with a destination that fails, then Closer returns the write error",
			destination: failingWriter{},
			key:         newKey(t),
			closeErr:    errWriteFailed,
		},
		{
			name:        "with a destination that does not make progress, then Closer returns a short write error",
			destination: stalledWriter{},
			key:         newKey(t),
			closeErr:    io.ErrShortWrite,
		},
	}

	for _, tt := range tests {
		t.Run(`Given an AESGCMHandler,
		when Encode method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			encoder, err := encrypt.NewAESGCMHandler().Encode(tt.destination, tt.key)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.ErrorIs(t, encoder.Closer(), tt.closeErr)
		})
	}
}
