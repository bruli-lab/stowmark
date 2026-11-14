package app

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

type restorer struct {
	objectRepo  ObjectRepository
	destination RestoreDestination
}

func (r restorer) restoreSnapshot(ctx context.Context, man *snapshot.Manifest, codec repository.Codec, destinationPath *string) *snapshot.Result {
	codec = codec.WithCompression(man.Compression())
	plan := snapshot.NewRestorePlan(man.Files())
	result := snapshot.NewResult(man.ID())
	record := func(entry snapshot.File, err error) {
		if err != nil {
			result.AddFailed(*snapshot.NewFailedResult(entry.Path(), err.Error()))
			return
		}
		result.AddSuccess()
	}

	created := make(map[string]bool, len(plan.Directories()))
	for _, entry := range plan.Directories() {
		err := r.withEntry(man, entry, destinationPath, func(target snapshot.File) error {
			return r.destination.CreateDirectory(ctx, target.Path())
		})
		created[entry.Path()] = err == nil
		if err != nil {
			record(entry, err)
		}
	}
	for _, entry := range plan.Files() {
		record(entry, r.withEntry(man, entry, destinationPath, func(target snapshot.File) error {
			return r.restoreContent(ctx, target, codec)
		}))
	}
	for _, entry := range plan.Symlinks() {
		record(entry, r.withEntry(man, entry, destinationPath, func(target snapshot.File) error {
			return r.destination.CreateSymlink(ctx, target.Path(), target.Target())
		}))
	}
	for _, entry := range plan.DirectoriesDeepestFirst() {
		if !created[entry.Path()] {
			continue
		}
		record(entry, r.withEntry(man, entry, destinationPath, func(target snapshot.File) error {
			return r.applyMetadata(ctx, target)
		}))
	}
	return result
}

func (r restorer) restoreEntry(ctx context.Context, man *snapshot.Manifest, entry snapshot.File, codec repository.Codec, destinationPath *string) error {
	codec = codec.WithCompression(man.Compression())
	return r.withEntry(man, entry, destinationPath, func(target snapshot.File) error {
		switch target.Kind() {
		case snapshot.DirectoryEntry:
			if err := r.destination.CreateDirectory(ctx, target.Path()); err != nil {
				return err
			}
			return r.applyMetadata(ctx, target)
		case snapshot.SymlinkEntry:
			return r.destination.CreateSymlink(ctx, target.Path(), target.Target())
		default:
			return r.restoreContent(ctx, target, codec)
		}
	})
}

func (r restorer) withEntry(man *snapshot.Manifest, entry snapshot.File, destinationPath *string, restore func(snapshot.File) error) error {
	if destinationPath == nil {
		return restore(entry)
	}
	relocated, err := entry.RelocatedTo(man.Source(), *destinationPath)
	if err != nil {
		return err
	}
	return restore(relocated)
}

func (r restorer) restoreContent(ctx context.Context, file snapshot.File, codec repository.Codec) error {
	if err := r.objectRepo.RestoreObject(ctx, &file, codec); err != nil {
		return err
	}
	return r.applyMetadata(ctx, file)
}

func (r restorer) applyMetadata(ctx context.Context, file snapshot.File) error {
	metadata, ok := file.Metadata()
	if !ok {
		return nil
	}
	return r.destination.ApplyMetadata(ctx, file.Path(), metadata)
}
