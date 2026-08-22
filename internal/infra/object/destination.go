package object

import (
	"path"
	"path/filepath"
	"strconv"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

const ObjectsFolder = "objects"

type ObjectsPath struct {
	DirectoryPath string
	ObjectPath    string
}

func GetObjectsPath(repositoryPath, hash string, generation uint64, key []byte, isRemote bool) *ObjectsPath {
	folder := ObjectsFolder
	if len(key) > 0 {
		folder = path.Join(ObjectsFolder, repository.EncryptedFolder, strconv.FormatUint(generation, 10))
	}

	prefix := hash[:2]
	name := hash[2:]

	if isRemote {
		directoryPath := path.Join(repositoryPath, folder, prefix)

		return &ObjectsPath{
			DirectoryPath: directoryPath,
			ObjectPath:    path.Join(directoryPath, name),
		}
	}

	directoryPath := filepath.Join(repositoryPath, filepath.FromSlash(folder), prefix)

	return &ObjectsPath{
		DirectoryPath: directoryPath,
		ObjectPath:    filepath.Join(directoryPath, name),
	}
}
