package snapshot_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestRestore_Restore(t *testing.T) {
	errTest := errors.New("test error")
	manifest := fixtures.ManifestBuilder{Files: []snapshot.File{
		fixtures.FileBuilder{}.Build(),
	}}.Build()
	type args struct {
		snapshotID string
	}
	tests := []struct {
		name string
		args args
		expectedErr, getConfigErr,
		manifestErr, restoreErr error
		manifest          *snapshot.Manifest
		expectedSuccess   bool
		expectedFailedLen int
	}{
		{
			name:        "and get manifest returns error, then it returns same error",
			manifestErr: errTest,
			expectedErr: errTest,
		},
		{
			name:              "and restore object returns error, then it returns a failed result",
			manifest:          &manifest,
			restoreErr:        errTest,
			expectedSuccess:   false,
			expectedFailedLen: 1,
		},
		{
			name:              "with no error, then it returns same error success result",
			manifest:          &manifest,
			expectedSuccess:   true,
			expectedFailedLen: 0,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a Restore service,
		when Restore method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			manifestRepo := &snapshot.ManifestRepositoryMock{}
			manifestRepo.GetFunc = func(_ context.Context, _ string) (*snapshot.Manifest, error) {
				return tt.manifest, tt.manifestErr
			}
			objectRepo := &snapshot.ObjectRepositoryMock{}
			objectRepo.RestoreObjectFunc = func(_ context.Context, _ *repository.Compression, _ *snapshot.File) error {
				return tt.restoreErr
			}

			svc := snapshot.NewRestore(manifestRepo, objectRepo)
			result, err := svc.Restore(t.Context(), tt.args.snapshotID)
			if err != nil {
				require.ErrorAs(t, err, &tt.expectedErr)
				return
			}
			if tt.expectedErr != nil {
				require.Error(t, err)
			}
			require.Equal(t, tt.args.snapshotID, result.SnapshotID())
			require.Equal(t, tt.expectedSuccess, result.IsSuccess())
			require.Len(t, result.Failed(), tt.expectedFailedLen)
		})
	}
}
