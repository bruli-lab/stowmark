package snapshot_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bruli-lab/stowmark.git/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark.git/internal/fixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestVerifier_Verify(t *testing.T) {
	errTest := errors.New("test error")
	hash := uuid.NewString()
	manifest := fixtures.ManifestBuilder{Files: []snapshot.File{
		fixtures.FileBuilder{Hash: &hash}.Build(),
	}}.Build()
	type args struct {
		snapshotID string
	}
	tests := []struct {
		name string
		args args
		expectedErr, getManifestErr,
		readObjectErr error
		expectedSuccess   bool
		expectedFailedLen int
		manifest          *snapshot.Manifest
		file              *snapshot.File
	}{
		{
			name:           "and get manifest returns error, then it returns same error",
			getManifestErr: errTest,
			expectedErr:    errTest,
		},
		{
			name:          "and read object returns error, then it returns same error",
			readObjectErr: errTest,
			expectedErr:   errTest,
			manifest:      &manifest,
		},
		{
			name:              "and read object returns not found error, then it returns a failed result",
			readObjectErr:     snapshot.NotFoundError{},
			manifest:          &manifest,
			expectedSuccess:   false,
			expectedFailedLen: 1,
		},
		{
			name:              "and read object returns with different hash, then it returns a failed result",
			manifest:          &manifest,
			expectedSuccess:   false,
			expectedFailedLen: 1,
			file:              new(fixtures.FileBuilder{}.Build()),
		},
		{
			name:              "and read object returns with different size, then it returns a failed result",
			manifest:          &manifest,
			expectedSuccess:   false,
			expectedFailedLen: 1,
			file:              new(fixtures.FileBuilder{Hash: new(hash), Size: new(int64(200))}.Build()),
		},
		{
			name:            "and read object returns same file data, then it returns a success result",
			manifest:        &manifest,
			expectedSuccess: true,
			file:            new(fixtures.FileBuilder{Hash: new(hash)}.Build()),
		},
	}
	for _, tt := range tests {
		t.Run(`Given a Verifier service,
		when Verify method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			objectRepo := &snapshot.ObjectRepositoryMock{}
			objectRepo.ReadObjectFunc = func(_ context.Context, _ string, _ string) (*snapshot.File, error) {
				return tt.file, tt.readObjectErr
			}
			manifestRepo := &snapshot.ManifestRepositoryMock{}
			manifestRepo.GetFunc = func(_ context.Context, _ string) (*snapshot.Manifest, error) {
				return tt.manifest, tt.getManifestErr
			}
			svc := snapshot.NewVerifier(objectRepo, manifestRepo)
			got, err := svc.Verify(t.Context(), tt.args.snapshotID)
			if err != nil {
				require.ErrorAs(t, err, &tt.expectedErr)
				return
			}
			if tt.expectedErr != nil {
				require.Error(t, err)
			}
			require.Equal(t, tt.expectedSuccess, got.IsSuccess())
			require.Equal(t, tt.args.snapshotID, got.SnapshotID())
			require.Len(t, got.Failed(), tt.expectedFailedLen)
		})
	}
}
