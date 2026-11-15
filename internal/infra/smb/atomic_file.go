package smb

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/cloudsoda/go-smb2"
)

const temporaryFilePrefix = "."

func writeFileAtomically(share *smb2.Share, filePath string, write func(io.Writer) error) error {
	temporaryPath, err := temporaryFilePath(filePath)
	if err != nil {
		return err
	}

	temporary, err := share.OpenFile(temporaryPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create SMB temporary file %q: %w", temporaryPath, err)
	}

	committed := false
	defer func() {
		if committed {
			return
		}
		_ = temporary.Close()
		_ = share.Remove(temporaryPath)
	}()

	if err := write(temporary); err != nil {
		return err
	}

	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync SMB temporary file %q: %w", temporaryPath, err)
	}

	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close SMB temporary file %q: %w", temporaryPath, err)
	}

	if err := share.Rename(temporaryPath, filePath); err != nil {
		return fmt.Errorf("commit SMB file %q: %w", filePath, err)
	}

	committed = true

	return nil
}

func writeBytesAtomically(share *smb2.Share, filePath string, data []byte) error {
	return writeFileAtomically(share, filePath, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
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
