package localfile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

const restoredDirectoryMode os.FileMode = 0o755

var ErrDirectoryInTheWay = errors.New("a directory already exists where the symlink is restored")

type RestoreDestination struct{}

func (RestoreDestination) CreateDirectory(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(path, restoredDirectoryMode); err != nil {
		return fmt.Errorf("create directory %q: %w", path, err)
	}
	return nil
}

func (RestoreDestination) CreateSymlink(ctx context.Context, path, target string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, restoredDirectoryMode); err != nil {
		return fmt.Errorf("create directory for symlink %q: %w", path, err)
	}
	if info, err := os.Lstat(path); err == nil && info.IsDir() {
		return fmt.Errorf("%w: %q", ErrDirectoryInTheWay, path)
	}

	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return fmt.Errorf("generate temporary symlink name: %w", err)
	}
	temporary := filepath.Join(directory, TemporaryFilePrefix+filepath.Base(path)+".tmp-"+hex.EncodeToString(suffix))
	if err := os.Symlink(target, temporary); err != nil {
		return fmt.Errorf("create symlink %q: %w", path, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("commit symlink %q: %w", path, err)
	}
	return nil
}

func (RestoreDestination) ApplyMetadata(ctx context.Context, path string, metadata snapshot.Metadata) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Chmod(path, metadata.Mode()); err != nil {
		return fmt.Errorf("set permissions of %q: %w", path, err)
	}
	if metadata.ModTime().IsZero() {
		return nil
	}
	if err := os.Chtimes(path, metadata.ModTime(), metadata.ModTime()); err != nil {
		return fmt.Errorf("set modification time of %q: %w", path, err)
	}
	return nil
}

func NewRestoreDestination() RestoreDestination {
	return RestoreDestination{}
}
