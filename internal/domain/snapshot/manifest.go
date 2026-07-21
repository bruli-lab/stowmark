package snapshot

import (
	"time"
)

type Manifest struct {
	id        string
	files     []File
	createdAt time.Time
	source    string
}

func (m Manifest) Id() string {
	return m.id
}

func (m Manifest) Files() []File {
	return m.files
}

func (m Manifest) CreatedAt() time.Time {
	return m.createdAt
}

func (m Manifest) Source() string {
	return m.source
}

func NewManifest(files []File, createdAt time.Time, source string) *Manifest {
	return &Manifest{id: newID(), files: files, createdAt: createdAt, source: source}
}
