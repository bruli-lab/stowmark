package object

import (
	"path"
	"path/filepath"
	"strconv"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

func EncryptedGenerationPath(repositoryPath string, generation uint64, isRemote bool) string {
	switch isRemote {
	case true:
		return path.Join(
			repositoryPath,
			repository.ObjectsFolder,
			repository.EncryptedFolder,
			strconv.FormatUint(generation, 10))
	default:
	}
	return filepath.Join(
		repositoryPath,
		repository.ObjectsFolder,
		repository.EncryptedFolder,
		strconv.FormatUint(generation, 10))
}
