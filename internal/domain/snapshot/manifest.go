package snapshot

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

type Manifest struct {
	id          string
	files       []File
	createdAt   time.Time
	source      string
	compression *repository.Compression
}

func (m *Manifest) Compression() *repository.Compression {
	return m.compression
}

func (m *Manifest) ID() string {
	return m.id
}

func (m *Manifest) Files() []File {
	return m.files
}

func (m *Manifest) CreatedAt() time.Time {
	return m.createdAt
}

func (m *Manifest) Source() string {
	return m.source
}

func NewManifest(id string, files []File, createdAt time.Time, source string, comp *repository.Compression) *Manifest {
	return &Manifest{id: id, files: files, createdAt: createdAt, source: source, compression: comp}
}

var (
	ErrFileNotInSnapshot    = errors.New("file not found in snapshot")
	ErrUnsafeRestoreInPlace = errors.New("the snapshot source is not a safe place to restore in place")
)

func (m *Manifest) ValidateRestore(destination *string) error {
	if destination != nil {
		return nil
	}
	if !filepath.IsAbs(m.source) || filepath.Clean(m.source) != m.source || filepath.Dir(m.source) == m.source {
		return fmt.Errorf("%w: %q must be a clean absolute path other than the filesystem root; use a destination",
			ErrUnsafeRestoreInPlace, m.source)
	}
	return nil
}

func (m *Manifest) RestoreTarget(entry File, destination *string) (File, error) {
	if destination == nil {
		return entry.RelocatedTo(m.source, m.source)
	}
	return entry.RelocatedTo(m.source, *destination)
}

func (m *Manifest) FindFile(filePath string) (*File, error) {
	for _, f := range m.files {
		if f.Path() == filePath {
			return &f, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrFileNotInSnapshot, filePath)
}

func (m *Manifest) ReferencedHashes() []string {
	hashes := make([]string, 0, len(m.files))
	for i := range m.files {
		hashes = append(hashes, m.files[i].ObjectHashes()...)
	}
	return hashes
}

func (m *Manifest) ObjectSizes() map[string]int64 {
	sizes := make(map[string]int64, len(m.files))
	for i := range m.files {
		file := m.files[i]
		if !file.HasContent() {
			continue
		}
		if len(file.Chunks()) == 0 {
			sizes[file.Hash()] = file.Size()
			continue
		}
		for _, chunk := range file.Chunks() {
			sizes[chunk.Hash()] = chunk.Size()
		}
	}
	return sizes
}

func (m *Manifest) Resume() *ManifestResume {
	var (
		size  int64
		files int
	)
	for _, f := range m.files {
		if !f.HasContent() {
			continue
		}
		files++
		size += f.Size()
	}
	return NewManifestResume(m.id, m.createdAt, files, size, m.source)
}
