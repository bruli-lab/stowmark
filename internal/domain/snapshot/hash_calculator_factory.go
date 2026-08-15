package snapshot

import (
	"context"
	"errors"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

type HashCalculator interface {
	Calculate(ctx context.Context, file *File, comp *repository.Compression) (*File, error)
}

type HashCalculatorFactory struct {
	calculators map[int]HashCalculator
}

func (h *HashCalculatorFactory) Handle(formatVersion int) (HashCalculator, error) {
	cal, ok := h.calculators[formatVersion]
	if !ok {
		return nil, errors.New("unknown format version")
	}
	return cal, nil
}

func NewHashCalculatorFactory(sourceRepo SourceRepository) *HashCalculatorFactory {
	calculators := map[int]HashCalculator{
		repository.OneFormatVersion: NewOneFormatVersionHashCalculator(sourceRepo),
		repository.TwoFormatVersion: NewTwoFormatVersionHashCalculator(sourceRepo),
	}
	return &HashCalculatorFactory{
		calculators: calculators,
	}
}

type OneFormatVersionHashCalculator struct {
	sourceRepo SourceRepository
}

func (o OneFormatVersionHashCalculator) Calculate(ctx context.Context, file *File, comp *repository.Compression) (*File, error) {
	hash, err := o.sourceRepo.CalculateHash(ctx, file.Path(), comp)
	if err != nil {
		return nil, err
	}
	file.AddHash(hash)
	return file, nil
}

func NewOneFormatVersionHashCalculator(sourceRepo SourceRepository) *OneFormatVersionHashCalculator {
	return &OneFormatVersionHashCalculator{sourceRepo: sourceRepo}
}

type TwoFormatVersionHashCalculator struct {
	sourceRepo SourceRepository
}

func (t TwoFormatVersionHashCalculator) Calculate(ctx context.Context, file *File, comp *repository.Compression) (*File, error) {
	var chunks []Chunk
	size := file.Size()
	switch {
	case size >= ChunkThreshold:
		split, err := t.sourceRepo.CalculateChunks(ctx, file.Path(), ChunkSize, comp)
		if err != nil {
			return nil, err
		}
		chunks = split
	default:
		hash, err := t.sourceRepo.CalculateHash(ctx, file.Path(), comp)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, *NewChunk(hash, 0, size))
		file.AddHash(hash)
	}
	file.AddChunks(chunks)
	return file, nil
}

func NewTwoFormatVersionHashCalculator(sourceRepo SourceRepository) *TwoFormatVersionHashCalculator {
	return &TwoFormatVersionHashCalculator{sourceRepo: sourceRepo}
}
