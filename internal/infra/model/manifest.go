package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

type Chunk struct {
	Hash   string `json:"hash"`
	Size   int64  `json:"size"`
	Offset int64  `json:"offset"`
}

type File struct {
	Path    string     `json:"path"`
	Kind    string     `json:"kind,omitempty"`
	Hash    string     `json:"hash,omitempty"`
	Size    int64      `json:"size"`
	Chunks  []Chunk    `json:"chunks,omitempty"`
	Target  string     `json:"target,omitempty"`
	Mode    *uint32    `json:"mode,omitempty"`
	ModTime *time.Time `json:"mtime,omitempty"`
}

type Manifest struct {
	ID          string      `json:"id"`
	Files       []File      `json:"files"`
	CreatedAt   time.Time   `json:"created_at"`
	Source      string      `json:"source"`
	Compression Compression `json:"compression"`
}

func BuildManifestDomain(data []byte, manifestPath string) (*snapshot.Manifest, error) {
	var mo Manifest
	if err := json.Unmarshal(data, &mo); err != nil {
		return nil, fmt.Errorf(
			"failed to unmarshal manifest %q: %w",
			manifestPath,
			err,
		)
	}
	snapshotFiles := make(
		[]snapshot.File,
		len(mo.Files),
	)
	for i, modelFile := range mo.Files {
		file, err := modelFile.domain()
		if err != nil {
			return nil, fmt.Errorf("failed to read manifest %q: %w", manifestPath, err)
		}
		snapshotFiles[i] = file
	}
	compType, err := repository.ParseCompressionType(mo.Compression.Type)
	if err != nil {
		return nil, err
	}
	comp, err := repository.NewCompression(*compType, mo.Compression.Level)
	if err != nil {
		return nil, err
	}
	man := snapshot.NewManifest(
		mo.ID,
		snapshotFiles,
		mo.CreatedAt,
		mo.Source,
		comp,
	)
	return man, nil
}

func NewManifest(m *snapshot.Manifest) Manifest {
	files := make([]File, len(m.Files()))
	for i, f := range m.Files() {
		chunks := make([]Chunk, len(f.Chunks()))
		for j, chunk := range f.Chunks() {
			chunks[j] = Chunk{
				Hash:   chunk.Hash(),
				Size:   chunk.Size(),
				Offset: chunk.Offset(),
			}
		}
		files[i] = File{
			Path:   f.Path(),
			Hash:   f.Hash(),
			Size:   f.Size(),
			Chunks: chunks,
			Target: f.Target(),
		}
		if f.Kind() != snapshot.RegularEntry {
			files[i].Kind = string(f.Kind())
		}
		if metadata, ok := f.Metadata(); ok {
			mode := unixMode(metadata.Mode())
			modTime := metadata.ModTime()
			files[i].Mode = &mode
			files[i].ModTime = &modTime
		}
	}
	man := Manifest{
		ID:        m.ID(),
		Files:     files,
		CreatedAt: m.CreatedAt().In(time.Local),
		Source:    m.Source(),
		Compression: Compression{
			Type:  m.Compression().CompType().String(),
			Level: m.Compression().Level(),
		},
	}
	return man
}
