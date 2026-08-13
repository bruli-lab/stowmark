package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

type File struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
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
		file := snapshot.File{}
		file.Hydrate(modelFile.Path, modelFile.Hash, modelFile.Size)
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
