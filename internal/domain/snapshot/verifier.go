package snapshot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

var defaultVerifyWorkers = runtime.GOMAXPROCS(0) * 4

type Verifier struct {
	objectRepo          ObjectRepository
	manifestRepo        ManifestRepository
	workers             int
	symmetricKeyHandler *SymmetricKeyHandler
}

func (v Verifier) Verify(ctx context.Context, repositoryPath, snapshotID string, privateKeyPath *string) (*Result, error) {
	data, err := v.symmetricKeyHandler.Handle(ctx, privateKeyPath, repositoryPath)
	if err != nil {
		return nil, err
	}
	var (
		symmetricKey []byte
		generation   uint64
	)
	if data != nil {
		symmetricKey = data.SymmetricKey
		generation = data.Generation
	}

	manifest, err := v.manifestRepo.Get(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	result := NewResult(snapshotID)

	files := manifest.Files()

	workers := v.workers
	if workers <= 0 {
		workers = defaultVerifyWorkers
	}
	if workers > len(files) {
		workers = len(files)
	}
	if workers < 1 {
		workers = 1
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan File)

	worker := func() {
		defer wg.Done()
		for file := range jobs {
			reason, err := v.verifyFile(ctx, &file, symmetricKey, generation)

			mu.Lock()
			switch {
			case err != nil:
				if firstErr == nil {
					firstErr = err
					cancel()
				}
			case reason != "":
				result.AddFailed(*NewFailedResult(file.Path(), reason))
			default:
				result.AddSuccess()
			}
			mu.Unlock()
		}
	}

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go worker()
	}

	for _, file := range files {
		select {
		case jobs <- file:
		case <-ctx.Done():
			goto drained
		}
	}
drained:
	close(jobs)
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	return result, nil
}

func (v Verifier) verifyFile(ctx context.Context, file *File, symmetricKey []byte, generation uint64) (string, error) {
	if len(file.Chunks()) == 0 {
		return v.verifyObject(
			ctx,
			file.Hash(),
			symmetricKey,
			generation,
		)
	}
	chunks := file.Chunks()
	for index, chunk := range chunks {
		reason, err := v.verifyObject(ctx, chunk.Hash(), symmetricKey, generation)
		if err != nil {
			return "", err
		}
		if reason != "" {
			return fmt.Sprintf("chunk %d/%d: %s", index+1, len(chunks), reason), nil
		}
	}

	return "", nil
}

func (v Verifier) verifyObject(ctx context.Context, expectedHash string, symmetricKey []byte, generation uint64) (string, error) {
	if expectedHash == "" {
		return "object hash is empty", nil
	}
	reader, err := v.objectRepo.ReadObject(ctx, expectedHash, symmetricKey, generation)
	if err != nil {
		if errors.As(err, &NotFoundError{}) {
			return err.Error(), nil
		}
		return "", err
	}

	hasher := sha256.New()

	_, copyErr := io.Copy(hasher, &contextReader{
		ctx:    ctx,
		reader: reader,
	})

	closeErr := reader.Close()

	if copyErr != nil {
		return "", fmt.Errorf("read object %q: %w", expectedHash, copyErr)
	}

	if closeErr != nil {
		return "", fmt.Errorf("close object %q: %w", expectedHash, closeErr)
	}

	calculatedHash := hex.EncodeToString(
		hasher.Sum(nil),
	)

	if calculatedHash != expectedHash {
		return fmt.Sprintf("hash mismatch: expected %s, calculated %s", expectedHash, calculatedHash), nil
	}

	return "", nil
}

func NewVerifier(
	objectRepo ObjectRepository,
	manifestRepo ManifestRepository,
	folderRepo repository.FolderRepository,
	decryptKeySvc *encryption.DecryptSymmetricKey,
) *Verifier {
	return &Verifier{
		objectRepo:          objectRepo,
		manifestRepo:        manifestRepo,
		workers:             defaultVerifyWorkers,
		symmetricKeyHandler: newSymmetricKeyHandler(folderRepo, decryptKeySvc),
	}
}
