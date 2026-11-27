package app

import (
	"context"
	"errors"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

var ErrUnknownCalculator = errors.New("unknown calculator")

type hashCalculator func(ctx context.Context, file snapshot.File, codec repository.Codec) (snapshot.File, error)

func newHashCalculator(sourceRepo SourceRepository, version repository.FormatVersion) (hashCalculator, error) {
	switch version {
	case repository.FormatVersionOne:
		return contentOnly(wholeFileHashCalculator(sourceRepo)), nil
	case repository.FormatVersionTwo, repository.FormatVersionThree:
		return contentOnly(chunkedHashCalculator(sourceRepo)), nil
	default:
		return nil, ErrUnknownCalculator
	}
}

func contentOnly(calculate hashCalculator) hashCalculator {
	return func(ctx context.Context, file snapshot.File, codec repository.Codec) (snapshot.File, error) {
		if !file.HasContent() {
			return file, nil
		}
		return calculate(ctx, file, codec)
	}
}

func wholeFileHashCalculator(sourceRepo SourceRepository) hashCalculator {
	return func(ctx context.Context, file snapshot.File, codec repository.Codec) (snapshot.File, error) {
		hash, err := sourceRepo.CalculateHash(ctx, file.Path(), codec)
		if err != nil {
			return snapshot.File{}, err
		}
		return file.WithHash(hash), nil
	}
}

func chunkedHashCalculator(sourceRepo SourceRepository) hashCalculator {
	return func(ctx context.Context, file snapshot.File, codec repository.Codec) (snapshot.File, error) {
		if file.Size() >= snapshot.ChunkThreshold {
			chunks, err := sourceRepo.CalculateChunks(ctx, file.Path(), snapshot.ChunkSize, codec)
			if err != nil {
				return snapshot.File{}, err
			}
			return file.WithChunks(chunks), nil
		}
		hash, err := sourceRepo.CalculateHash(ctx, file.Path(), codec)
		if err != nil {
			return snapshot.File{}, err
		}
		if file.Size() == 0 {
			return file.WithHash(hash).WithChunks(nil), nil
		}
		return file.WithHash(hash).WithChunks([]snapshot.Chunk{*snapshot.NewChunk(hash, 0, file.Size())}), nil
	}
}
