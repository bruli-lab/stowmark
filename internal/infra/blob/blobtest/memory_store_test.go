package blobtest_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"github.com/bruli-lab/stowmark/internal/infra/blob/blobtest"
)

func TestMemoryStore(t *testing.T) {
	t.Parallel()

	blobtest.RunStoreContract(t, func(*testing.T) blob.Store {
		return blobtest.NewMemoryStore()
	})
}
