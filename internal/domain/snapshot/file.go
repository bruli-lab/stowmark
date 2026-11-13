package snapshot

import (
	"errors"
	"fmt"
	"path/filepath"
)

var ErrPathOutsideSource = errors.New("file path is outside the snapshot source")

type File struct {
	path   string
	size   int64
	hash   string
	chunks []Chunk
}

func (f *File) Path() string {
	return f.path
}

func (f *File) Size() int64 {
	return f.size
}

func (f *File) Hash() string {
	return f.hash
}

func (f *File) AddHash(hash string) {
	f.hash = hash
}

func (f *File) Chunks() []Chunk {
	return f.chunks
}

func (f *File) AddChunks(c []Chunk) {
	f.chunks = c
}

func (f *File) ChangeSourcePath(source, destination string) error {
	relativePath, err := filepath.Rel(source, f.path)
	if err != nil || !filepath.IsLocal(relativePath) {
		return fmt.Errorf("%w: %q is not inside %q", ErrPathOutsideSource, f.path, source)
	}
	f.path = filepath.Join(destination, relativePath)
	return nil
}

func (f *File) Hydrate(path, hash string, size int64, chunks []Chunk) {
	f.size = size
	f.path = path
	f.hash = hash
	f.chunks = chunks
}

func NewFile(path string, size int64) *File {
	return &File{path: path, size: size}
}
