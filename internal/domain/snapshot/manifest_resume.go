package snapshot

import "time"

type ManifestResume struct {
	id        string
	createdAt time.Time
	files     int
	size      int64
	source    string
}

func (m ManifestResume) Id() string {
	return m.id
}

func (m ManifestResume) CreatedAt() time.Time {
	return m.createdAt
}

func (m ManifestResume) Files() int {
	return m.files
}

func (m ManifestResume) Size() int64 {
	return m.size
}

func (m ManifestResume) Source() string {
	return m.source
}

func NewManifestResume(id string, createdAt time.Time, files int, size int64, source string) *ManifestResume {
	return &ManifestResume{id: id, createdAt: createdAt, files: files, size: size, source: source}
}
