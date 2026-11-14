package app_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

const largeObjectSize int64 = 400 << 20

type concurrentReads struct {
	active  atomic.Int32
	maximum atomic.Int32
}

func (c *concurrentReads) read(contents map[string]string) func(context.Context, string, []byte, uint64) (io.ReadCloser, error) {
	return func(_ context.Context, hash string, _ []byte, _ uint64) (io.ReadCloser, error) {
		active := c.active.Add(1)
		defer c.active.Add(-1)
		for {
			current := c.maximum.Load()
			if active <= current || c.maximum.CompareAndSwap(current, active) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		return io.NopCloser(strings.NewReader(contents[hash])), nil
	}
}

func largeManifest(t *testing.T) (manifest *snapshot.Manifest, contents map[string]string) {
	t.Helper()
	contents = make(map[string]string)
	files := make([]snapshot.File, 4)
	for i := range files {
		content := fmt.Sprintf("large-%d", i)
		hash := hashOf(content)
		contents[hash] = content
		files[i] = fixtures.FileBuilder{Path: new(fmt.Sprintf("/source/large-%d", i)), Hash: &hash, Size: new(largeObjectSize)}.Build()
	}
	built := fixtures.ManifestBuilder{Files: files}.Build()
	return &built, contents
}

func TestVerifySnapshot_Handle_MemoryBudget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		encryption      *encryption.EncryptionConfig
		privateKey      *string
		expectedMaximum int32
	}{
		{
			name:            "on an encrypted repository, then large objects are read one at a time",
			encryption:      encryption.NewEncryptionConfig("key", "fingerprint", 1),
			privateKey:      new("private.pem"),
			expectedMaximum: 1,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a VerifySnapshot query handler with several workers and objects larger than the memory budget,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			manifest, contents := largeManifest(t)
			config := fixtures.ConfigBuilder{EncryptionConfig: tt.encryption}.Build(t)
			reads := &concurrentReads{}
			asymmetric, symmetric := keyRepositories()
			handler := app.NewVerifySnapshotWithWorkers(
				&app.ManifestRepositoryMock{GetFunc: func(context.Context, string) (*snapshot.Manifest, error) { return manifest, nil }},
				&app.ObjectRepositoryMock{ReadObjectFunc: reads.read(contents)},
				&app.FolderRepositoryMock{GetConfigFunc: func(context.Context, string) (*repository.Config, error) { return &config, nil }},
				asymmetric, symmetric, newUnlockedLocker(), 4,
			)
			query := verifyQuery
			query.PrivateKeyPath = tt.privateKey

			result, err := verifySnapshot(t.Context(), handler, query)

			require.NoError(t, err)
			require.True(t, result.IsSuccess())
			require.Equal(t, tt.expectedMaximum, reads.maximum.Load())
		})
	}

	t.Run(`Given a VerifySnapshot query handler with several workers on a plain repository,
	when Handle method is called with objects larger than the memory budget,
	then the objects are still read in parallel because they are streamed`, func(t *testing.T) {
		t.Parallel()

		manifest, contents := largeManifest(t)
		config := fixtures.ConfigBuilder{}.Build(t)
		reads := &concurrentReads{}
		handler := app.NewVerifySnapshotWithWorkers(
			&app.ManifestRepositoryMock{GetFunc: func(context.Context, string) (*snapshot.Manifest, error) { return manifest, nil }},
			&app.ObjectRepositoryMock{ReadObjectFunc: reads.read(contents)},
			&app.FolderRepositoryMock{GetConfigFunc: func(context.Context, string) (*repository.Config, error) { return &config, nil }},
			&app.AsymmetricKeyPairRepositoryMock{}, &app.SymmetricKeyRepositoryMock{}, newUnlockedLocker(), 4,
		)

		result, err := verifySnapshot(t.Context(), handler, verifyQuery)

		require.NoError(t, err)
		require.True(t, result.IsSuccess())
		require.Greater(t, reads.maximum.Load(), int32(1))
	})
}
