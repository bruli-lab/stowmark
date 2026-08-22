package encrypt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sync"
)

type Encoder struct {
	Writer io.Writer
	Closer func() error
}

type AESGCMHandler struct{}

func NewAESGCMHandler() *AESGCMHandler {
	return &AESGCMHandler{}
}

func (h AESGCMHandler) Decode(source io.Reader, key []byte) (io.Reader, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM cipher: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(source, nonce); err != nil {
		return nil, fmt.Errorf("read nonce: %w", err)
	}

	ciphertext, err := io.ReadAll(source)
	if err != nil {
		return nil, fmt.Errorf("read encrypted data: %w", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt data: %w", err)
	}

	return bytes.NewReader(plaintext), nil
}

func (h AESGCMHandler) Encode(destination io.Writer, key []byte) (*Encoder, error) {
	if destination == nil {
		return nil, errors.New("encryption destination is required")
	}

	if len(key) != AES256KeySize {
		return nil, fmt.Errorf("invalid AES-256 key size: got %d bytes, expected %d", len(key), AES256KeySize)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create AES-GCM cipher: %w", err)
	}

	var buffer bytes.Buffer
	var closeOnce sync.Once
	var closeErr error

	return &Encoder{
		Writer: &buffer,
		Closer: func() error {
			closeOnce.Do(func() {
				nonce := make([]byte, aead.NonceSize())
				if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
					closeErr = fmt.Errorf("generate AES-GCM nonce: %w", err)
					return
				}

				encrypted := aead.Seal(nil, nonce, buffer.Bytes(), nil)

				if err := writeFull(destination, nonce); err != nil {
					closeErr = fmt.Errorf("write AES-GCM nonce: %w", err)
					return
				}

				if err := writeFull(destination, encrypted); err != nil {
					closeErr = fmt.Errorf("write encrypted data: %w", err)
				}
			})

			return closeErr
		},
	}, nil
}

func writeFull(destination io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := destination.Write(data)
		if err != nil {
			return err
		}

		if n == 0 {
			return io.ErrShortWrite
		}

		data = data[n:]
	}

	return nil
}
