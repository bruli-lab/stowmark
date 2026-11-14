package model

import (
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

var ErrUnknownEntryKind = errors.New("unknown manifest entry kind")

const (
	unixSetuid uint32 = 0o4000
	unixSetgid uint32 = 0o2000
	unixSticky uint32 = 0o1000
)

func (f *File) domain() (snapshot.File, error) {
	var file snapshot.File
	switch snapshot.EntryKind(f.Kind) {
	case "", snapshot.RegularEntry:
		chunks := make([]snapshot.Chunk, len(f.Chunks))
		for j, chunk := range f.Chunks {
			chunks[j] = *snapshot.NewChunk(chunk.Hash, chunk.Offset, chunk.Size)
		}
		file = *snapshot.HydrateFile(f.Path, f.Hash, f.Size, chunks)
	case snapshot.DirectoryEntry:
		file = *snapshot.NewDirectory(f.Path)
	case snapshot.SymlinkEntry:
		file = *snapshot.NewSymlink(f.Path, f.Target)
	default:
		return snapshot.File{}, fmt.Errorf("%w: %q", ErrUnknownEntryKind, f.Kind)
	}
	if f.Mode == nil && f.ModTime == nil {
		return file, nil
	}
	var modTime time.Time
	if f.ModTime != nil {
		modTime = *f.ModTime
	}
	var mode fs.FileMode
	if f.Mode != nil {
		mode = fileMode(*f.Mode)
	}
	return file.WithMetadata(snapshot.NewMetadata(mode, modTime)), nil
}

func UnixMode(mode fs.FileMode) uint32 {
	unix := uint32(mode.Perm())
	if mode&fs.ModeSetuid != 0 {
		unix |= unixSetuid
	}
	if mode&fs.ModeSetgid != 0 {
		unix |= unixSetgid
	}
	if mode&fs.ModeSticky != 0 {
		unix |= unixSticky
	}
	return unix
}

func fileMode(unix uint32) fs.FileMode {
	mode := fs.FileMode(unix) & fs.ModePerm
	if unix&unixSetuid != 0 {
		mode |= fs.ModeSetuid
	}
	if unix&unixSetgid != 0 {
		mode |= fs.ModeSetgid
	}
	if unix&unixSticky != 0 {
		mode |= fs.ModeSticky
	}
	return mode
}
