package blob

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

const (
	manifestExtension = ".json"
	minimumHashLength = 3
)

func ValidateKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") || path.Clean(key) != key {
		return fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	for segment := range strings.SplitSeq(key, "/") {
		if segment == ".." || strings.HasPrefix(segment, ".") {
			return fmt.Errorf("%w: %q", ErrInvalidKey, key)
		}
	}
	return nil
}

func validateHash(hash string) error {
	if len(hash) < minimumHashLength {
		return fmt.Errorf("%w: %q", ErrInvalidHash, hash)
	}
	for _, char := range hash {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return fmt.Errorf("%w: %q", ErrInvalidHash, hash)
		}
	}
	return nil
}

func objectsPrefix(encrypted bool, generation uint64) string {
	if !encrypted {
		return repository.ObjectsFolder
	}
	return generationPrefix(generation)
}

func generationPrefix(generation uint64) string {
	return path.Join(repository.ObjectsFolder, repository.EncryptedFolder, strconv.FormatUint(generation, 10))
}

func objectKey(hash string, encrypted bool, generation uint64) (string, error) {
	if err := validateHash(hash); err != nil {
		return "", err
	}
	return path.Join(objectsPrefix(encrypted, generation), hash[:2], hash[2:]), nil
}

func hashFromKey(prefix, key string) (string, bool) {
	relative, ok := strings.CutPrefix(key, prefix+"/")
	if !ok {
		return "", false
	}
	directory, name, ok := strings.Cut(relative, "/")
	if !ok || len(directory) != 2 || name == "" || strings.Contains(name, "/") {
		return "", false
	}
	hash := directory + name
	if validateHash(hash) != nil {
		return "", false
	}
	return hash, true
}

func manifestKey(snapshotID string) (string, error) {
	key := path.Join(repository.SnapshotsFolder, snapshotID+manifestExtension)
	if strings.Contains(snapshotID, "/") {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	return key, nil
}

func isManifestKey(key string) bool {
	relative, ok := strings.CutPrefix(key, repository.SnapshotsFolder+"/")
	return ok && !strings.Contains(relative, "/") && strings.HasSuffix(relative, manifestExtension)
}
