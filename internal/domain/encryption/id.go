package encryption

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func generateKeyID() (string, error) {
	data := make([]byte, 4)

	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate key ID: %w", err)
	}

	return hex.EncodeToString(data), nil
}
