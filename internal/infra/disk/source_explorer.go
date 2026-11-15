package disk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/object"
)

type SourceExplorer struct {
	handlersFactory *compression.HandlersFactory
}

func (s SourceExplorer) CalculateChunks(ctx context.Context, filePath string, chunkSize int64, codec repository.Codec) ([]snapshot.Chunk, error) {
	if chunkSize <= 0 {
		return nil, fmt.Errorf("chunk size must be greater than zero: %d", chunkSize)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file %q: %w", filePath, err)
	}
	defer func() {
		_ = file.Close()
	}()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file %q: %w", filePath, err)
	}

	fileSize := info.Size()
	chunks := make([]snapshot.Chunk, 0, int((fileSize+chunkSize-1)/chunkSize))

	for offset := int64(0); offset < fileSize; offset += chunkSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		currentSize := min(chunkSize, fileSize-offset)
		hash, err := object.Digest(ctx, io.NewSectionReader(file, offset, currentSize), codec, s.handlersFactory)
		if err != nil {
			return nil, fmt.Errorf("calculate chunk hash for %q at offset %d: %w", filePath, offset, err)
		}

		chunks = append(chunks, *snapshot.NewChunk(hash, offset, currentSize))
	}

	return chunks, nil
}

func (s SourceExplorer) CalculateHash(ctx context.Context, filePath string, codec repository.Codec) (string, error) {
	fi, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file %q: %w", filePath, err)
	}
	defer func() {
		_ = fi.Close()
	}()

	return object.Digest(ctx, fi, codec, s.handlersFactory)
}

func (s SourceExplorer) IgnorePatterns(ctx context.Context, sourcePath string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ignoreFile := filepath.Join(sourcePath, snapshot.IgnoreFileName)
	data, err := os.ReadFile(ignoreFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ignore file %q: %w", ignoreFile, err)
	}
	return strings.Split(string(data), "\n"), nil
}

func (s SourceExplorer) Explore(ctx context.Context, sourcePath string, rules *snapshot.ExcludeRules) (*snapshot.Source, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	absolutePath, err := absolutePath(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("resolve absolute path: %w", err)
	}

	info, err := os.Stat(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("stat source path: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("source path is not a directory: %s", absolutePath)
	}
	files, skipped, err := s.readFiles(ctx, absolutePath, rules)
	if err != nil {
		return nil, fmt.Errorf("read files: %w", err)
	}
	return snapshot.NewSource(absolutePath, files, skipped), nil
}

func (s SourceExplorer) readFiles(ctx context.Context, root string, rules *snapshot.ExcludeRules) (files []snapshot.File, skipped []snapshot.SkippedFile, err error) {
	files = make([]snapshot.File, 0)
	directoryRules := map[string]*snapshot.ExcludeRules{".": rules}

	err = filepath.WalkDir(root, func(
		path string,
		dirEntry fs.DirEntry,
		walkErr error,
	) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		slashed := filepath.ToSlash(relative)
		parentRules := directoryRules[pathpkg.Dir(slashed)]
		if parentRules != nil && parentRules.Excludes(slashed, dirEntry.IsDir()) {
			if dirEntry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if dirEntry.IsDir() {
			nested, err := s.nestedRules(ctx, path, slashed, parentRules)
			if err != nil {
				return err
			}
			directoryRules[slashed] = nested
		}

		entry, skip, err := readEntry(path, relative, dirEntry.Type())
		if err != nil {
			return err
		}
		if skip != nil {
			skipped = append(skipped, *skip)
			return nil
		}
		files = append(files, entry)

		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("walk directory %q: %w", root, err)
	}
	return files, skipped, nil
}

func (s SourceExplorer) nestedRules(ctx context.Context, directory, relative string, parent *snapshot.ExcludeRules) (*snapshot.ExcludeRules, error) {
	if parent == nil || !parent.ReadsIgnoreFiles() {
		return parent, nil
	}
	lines, err := s.IgnorePatterns(ctx, directory)
	if err != nil {
		return nil, err
	}
	return parent.WithIgnoreFile(relative, lines)
}

func readEntry(path, relative string, mode fs.FileMode) (snapshot.File, *snapshot.SkippedFile, error) {
	if !mode.IsDir() && !mode.IsRegular() && mode&fs.ModeSymlink == 0 {
		skipped := snapshot.NewSkippedFile(filepath.ToSlash(relative), skippedKind(mode))
		return snapshot.File{}, &skipped, nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return snapshot.File{}, nil, err
	}
	metadata := snapshot.NewMetadata(info.Mode(), info.ModTime())
	switch {
	case info.IsDir():
		return snapshot.NewDirectory(path).WithMetadata(metadata), nil, nil
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return snapshot.File{}, nil, fmt.Errorf("read symlink %q: %w", path, err)
		}
		return snapshot.NewSymlink(path, target).WithMetadata(metadata), nil, nil
	default:
		return snapshot.NewFile(path, info.Size()).WithMetadata(metadata), nil, nil
	}
}

func skippedKind(mode fs.FileMode) string {
	switch {
	case mode&fs.ModeSymlink != 0:
		return snapshot.SymlinkKind
	case mode&fs.ModeDevice != 0:
		return snapshot.DeviceKind
	case mode&fs.ModeSocket != 0:
		return snapshot.SocketKind
	case mode&fs.ModeNamedPipe != 0:
		return snapshot.NamedPipeKind
	default:
		return snapshot.IrregularKind
	}
}

func NewSourceRepository() *SourceExplorer {
	return &SourceExplorer{handlersFactory: compression.NewHandlersFactory()}
}
