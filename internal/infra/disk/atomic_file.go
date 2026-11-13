package disk

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const temporaryFilePrefix = "."

func writeFileAtomically(filePath string, perm os.FileMode, write func(io.Writer) error) error {
	directory := filepath.Dir(filePath)

	temporary, err := os.CreateTemp(directory, temporaryFilePrefix+filepath.Base(filePath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file in %q: %w", directory, err)
	}

	temporaryPath := temporary.Name()
	committed := false

	defer func() {
		if committed {
			return
		}
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()

	if err := write(temporary); err != nil {
		return err
	}

	if err := temporary.Chmod(perm); err != nil {
		return fmt.Errorf("set permissions on temporary file %q: %w", temporaryPath, err)
	}

	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary file %q: %w", temporaryPath, err)
	}

	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file %q: %w", temporaryPath, err)
	}

	if err := os.Rename(temporaryPath, filePath); err != nil {
		return fmt.Errorf("commit file %q: %w", filePath, err)
	}

	committed = true

	return syncDirectory(directory)
}

func writeBytesAtomically(filePath string, perm os.FileMode, data []byte) error {
	return writeFileAtomically(filePath, perm, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}

func syncDirectory(directory string) error {
	if runtime.GOOS == "windows" {
		return nil
	}

	dir, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open directory %q: %w", directory, err)
	}

	syncErr := dir.Sync()
	closeErr := dir.Close()

	if syncErr != nil {
		return fmt.Errorf("sync directory %q: %w", directory, syncErr)
	}

	return closeErr
}

func isTemporaryFile(name string) bool {
	return strings.HasPrefix(name, temporaryFilePrefix)
}
