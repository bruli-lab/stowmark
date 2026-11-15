package snapshot

import (
	"path/filepath"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

const (
	SymlinkKind   = "symbolic link"
	DeviceKind    = "device"
	SocketKind    = "socket"
	NamedPipeKind = "named pipe"
	IrregularKind = "irregular file"
)

type SkippedFile struct {
	path string
	kind string
}

func (s SkippedFile) Path() string {
	return s.path
}

func (s SkippedFile) Kind() string {
	return s.kind
}

func NewSkippedFile(path, kind string) SkippedFile {
	return SkippedFile{path: path, kind: kind}
}

type Source struct {
	absolutePath string
	files        []File
	skipped      []SkippedFile
}

func (s Source) AbsolutePath() string {
	return s.absolutePath
}

func (s Source) Files() []File {
	return s.files
}

func (s Source) Skipped() []SkippedFile {
	return s.skipped
}

func NewSource(absolutePath string, files []File, skipped []SkippedFile) *Source {
	return &Source{absolutePath: absolutePath, files: files, skipped: skipped}
}

func (s Source) ForFormat(version repository.FormatVersion) *Source {
	if version.ContentAddressed() {
		return &s
	}
	files := make([]File, 0, len(s.files))
	skipped := append(make([]SkippedFile, 0, len(s.skipped)), s.skipped...)
	for _, file := range s.files {
		switch file.Kind() {
		case RegularEntry:
			files = append(files, file.WithoutMetadata())
		case SymlinkEntry:
			skipped = append(skipped, NewSkippedFile(s.relative(file.Path()), SymlinkKind))
		case DirectoryEntry:
		}
	}
	return NewSource(s.absolutePath, files, skipped)
}

func (s Source) relative(path string) string {
	relative, err := filepath.Rel(s.absolutePath, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}
