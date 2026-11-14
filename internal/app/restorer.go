package app

import (
	"context"
	"runtime"
	"sync"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"golang.org/x/sync/errgroup"
)

var defaultRestoreWorkers = runtime.GOMAXPROCS(0) * 2

type restorer struct {
	objectRepo  ObjectRepository
	destination RestoreDestination
	budget      *memoryBudget
	workers     int
}

func newRestorer(objectRepo ObjectRepository, destination RestoreDestination, workers int) restorer {
	if workers <= 0 {
		workers = defaultRestoreWorkers
	}
	return restorer{
		objectRepo:  objectRepo,
		destination: destination,
		budget:      newMemoryBudget(defaultMemoryBudget),
		workers:     workers,
	}
}

func (r restorer) restoreSnapshot(
	ctx context.Context,
	man *snapshot.Manifest,
	codec repository.Codec,
	destinationPath *string,
	progress Progress,
) (*snapshot.Result, error) {
	codec = codec.WithCompression(man.Compression())
	plan := snapshot.NewRestorePlan(man.Files())
	result := snapshot.NewResult(man.ID())
	var mu sync.Mutex
	record := func(entry snapshot.File, err error) {
		mu.Lock()
		defer mu.Unlock()
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
	items, bytes := contentTotals(plan.Files())
	reporter := startProgress(progress, ProgressRestoring, items, bytes)
	err := r.restoreFiles(ctx, man, plan.Files(), codec, destinationPath, func(entry snapshot.File, err error) {
		record(entry, err)
		reporter.Advance(1, contentSize(entry))
	})
	reporter.Finish()
	if err != nil {
		return nil, err
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r restorer) restoreFiles(
	ctx context.Context,
	man *snapshot.Manifest,
	files []snapshot.File,
	codec repository.Codec,
	destinationPath *string,
	record func(snapshot.File, error),
) error {
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(r.workers)
	for _, entry := range files {
		if groupCtx.Err() != nil {
			break
		}
		group.Go(func() error {
			release, err := r.budget.reserve(groupCtx, codec, largestObject(entry))
			if err != nil {
				return err
			}
			defer release()
			record(entry, r.withEntry(man, entry, destinationPath, func(target snapshot.File) error {
				return r.restoreContent(groupCtx, target, codec)
			}))
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	return ctx.Err()
}

func largestObject(file snapshot.File) int64 {
	if len(file.Chunks()) == 0 {
		return file.Size()
	}
	var largest int64
	for _, chunk := range file.Chunks() {
		largest = max(largest, chunk.Size())
	}
	return largest
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
