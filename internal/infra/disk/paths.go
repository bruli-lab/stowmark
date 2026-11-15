package disk

import (
	"fmt"
	"path/filepath"
)

func absolutePath(repositoryPath string) (string, error) {
	absolutePath, err := filepath.Abs(repositoryPath)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	return absolutePath, nil
}
