package ssh

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/pkg/sftp"
)

const temporaryFilePrefix = "."

func writeFileAtomically(client *sftp.Client, filePath string, write func(io.Writer) error) error {
	temporaryPath, err := temporaryFilePath(filePath)
	if err != nil {
		return err
	}

	temporary, err := client.Create(temporaryPath)
	if err != nil {
		return fmt.Errorf("create remote temporary file %q: %w", temporaryPath, err)
	}

	committed := false
	defer func() {
		if committed {
			return
		}
		_ = temporary.Close()
		_ = client.Remove(temporaryPath)
	}()

	if err := write(temporary); err != nil {
		return err
	}

	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close remote temporary file %q: %w", temporaryPath, err)
	}

	if err := client.PosixRename(temporaryPath, filePath); err != nil {
		return fmt.Errorf("commit remote file %q: %w", filePath, err)
	}

	committed = true

	return nil
}

func temporaryFilePath(filePath string) (string, error) {
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("generate temporary file name: %w", err)
	}

	directory, name := path.Split(filePath)

	return path.Join(directory, temporaryFilePrefix+name+".tmp-"+hex.EncodeToString(suffix)), nil
}

func isTemporaryFile(name string) bool {
	return strings.HasPrefix(name, temporaryFilePrefix)
}
