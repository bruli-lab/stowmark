package snapshot

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
)

var ErrPathOutsideSource = errors.New("file path is outside the snapshot source")

type File struct {
	path   string
	size   int64
	hash   string
	chunks []Chunk
}

func (f File) Path() string {
	return f.path
}

func (f File) Size() int64 {
	return f.size
}

func (f File) Hash() string {
	return f.hash
}

func (f File) Chunks() []Chunk {
	return f.chunks
}

func (f File) WithHash(hash string) File {
	f.hash = hash
	return f
}

func (f File) WithChunks(chunks []Chunk) File {
	f.chunks = slices.Clone(chunks)
	return f
}

func (f File) RelocatedTo(source, destination string) (File, error) {
	relativePath, err := filepath.Rel(source, f.path)
	if err != nil || !filepath.IsLocal(relativePath) {
		return File{}, fmt.Errorf("%w: %q is not inside %q", ErrPathOutsideSource, f.path, source)
	}
	f.path = filepath.Join(destination, relativePath)
	return f, nil
}

func NewFile(path string, size int64) *File {
	return &File{path: path, size: size}
}

func HydrateFile(path, hash string, size int64, chunks []Chunk) *File {
	return &File{path: path, size: size, hash: hash, chunks: slices.Clone(chunks)}
}
