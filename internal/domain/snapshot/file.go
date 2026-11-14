package snapshot

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"time"
)

var ErrPathOutsideSource = errors.New("file path is outside the snapshot source")

type EntryKind string

const (
	RegularEntry   EntryKind = "file"
	DirectoryEntry EntryKind = "dir"
	SymlinkEntry   EntryKind = "symlink"
)

type Metadata struct {
	mode    fs.FileMode
	modTime time.Time
}

func (m Metadata) Mode() fs.FileMode {
	return m.mode
}

func (m Metadata) ModTime() time.Time {
	return m.modTime
}

func NewMetadata(mode fs.FileMode, modTime time.Time) Metadata {
	return Metadata{mode: mode & (fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky), modTime: modTime.UTC()}
}

type entryDetails struct {
	kind        EntryKind
	target      string
	metadata    Metadata
	hasMetadata bool
}

func (d *entryDetails) clone() entryDetails {
	if d == nil {
		return entryDetails{}
	}
	return *d
}

func (d entryDetails) isEmpty() bool {
	return d.kind == "" && d.target == "" && !d.hasMetadata
}

type File struct {
	path    string
	size    int64
	hash    string
	chunks  []Chunk
	details *entryDetails
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

func (f File) Kind() EntryKind {
	if f.details == nil || f.details.kind == "" {
		return RegularEntry
	}
	return f.details.kind
}

func (f File) Target() string {
	if f.details == nil {
		return ""
	}
	return f.details.target
}

func (f File) Metadata() (Metadata, bool) {
	if f.details == nil || !f.details.hasMetadata {
		return Metadata{}, false
	}
	return f.details.metadata, true
}

func (f File) HasContent() bool {
	return f.Kind() == RegularEntry
}

func (f File) ObjectHashes() []string {
	if !f.HasContent() {
		return nil
	}
	if len(f.chunks) == 0 {
		return []string{f.hash}
	}
	hashes := make([]string, len(f.chunks))
	for i, chunk := range f.chunks {
		hashes[i] = chunk.Hash()
	}
	return hashes
}

func (f File) WithHash(hash string) File {
	f.hash = hash
	return f
}

func (f File) WithChunks(chunks []Chunk) File {
	f.chunks = slices.Clone(chunks)
	return f
}

func (f File) WithMetadata(metadata Metadata) File {
	details := f.details.clone()
	details.metadata = metadata
	details.hasMetadata = true
	f.details = &details
	return f
}

func (f File) WithoutMetadata() File {
	details := f.details.clone()
	details.metadata = Metadata{}
	details.hasMetadata = false
	if details.isEmpty() {
		f.details = nil
		return f
	}
	f.details = &details
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

func NewDirectory(path string) *File {
	return &File{path: path, details: &entryDetails{kind: DirectoryEntry}}
}

func NewSymlink(path, target string) *File {
	return &File{path: path, details: &entryDetails{kind: SymlinkEntry, target: target}}
}

func HydrateFile(path, hash string, size int64, chunks []Chunk) *File {
	return &File{path: path, size: size, hash: hash, chunks: slices.Clone(chunks)}
}
