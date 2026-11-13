package disk

import (
	"io"
	"os"

	"github.com/bruli-lab/stowmark/internal/infra/localfile"
)

func writeFileAtomically(filePath string, perm os.FileMode, write func(io.Writer) error) error {
	return localfile.WriteAtomically(filePath, perm, write)
}

func writeBytesAtomically(filePath string, perm os.FileMode, data []byte) error {
	return localfile.WriteBytesAtomically(filePath, perm, data)
}

func isTemporaryFile(name string) bool {
	return localfile.IsTemporary(name)
}
