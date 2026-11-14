package snapshot

import (
	"errors"
	"fmt"
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

var ErrFileNotInSnapshot = errors.New("file not found in snapshot")

func (m *Manifest) FindFile(filePath string) (*File, error) {
	for _, f := range m.files {
		if f.Path() == filePath {
			return &f, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrFileNotInSnapshot, filePath)
}

func (m *Manifest) Resume() *ManifestResume {
	var size int64
	for _, f := range m.files {
		size += f.Size()
	}
	return NewManifestResume(m.id, m.createdAt, len(m.files), size, m.source)
}
