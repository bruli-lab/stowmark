package snapshot_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bruli-lab/stonekeep.git/internal/domain/repository"
	"github.com/bruli-lab/stonekeep.git/internal/domain/snapshot"
	"github.com/bruli-lab/stonekeep.git/internal/fixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCreate_Do(t *testing.T) {
	errTest := errors.New("test error")
	source := fixtures.SourceBuilder{Files: []snapshot.File{
		fixtures.FileBuilder{}.Build(),
	}}.Build()
	type args struct {
		repoPath   string
		sourcePath string
	}
	tests := []struct {
		name string
		args args
		expectedErr, getConfigErr,
		exploreErr, calculateHashErr,
		saveObjErr, alreadyExistsErr,
		saveManifestErr error
		source *snapshot.Source
		hash   string
		exists bool
	}{
		{
			name:         "and get config returns error, then it returns same error",
			getConfigErr: errTest,
			expectedErr:  errTest,
		},
		{
			name:        "and explore returns error, then it returns same error",
			exploreErr:  errTest,
			expectedErr: errTest,
		},
		{
			name:             "and calculate hash returns error, then it returns same error",
			calculateHashErr: errTest,
			expectedErr:      errTest,
			source:           &source,
		},
		{
			name:             "and already exists returns error, then it returns same error",
			alreadyExistsErr: errTest,
			expectedErr:      errTest,
			source:           &source,
			hash:             uuid.NewString(),
		},
		{
			name:        "and save object returns error, then it returns same error",
			saveObjErr:  errTest,
			expectedErr: errTest,
			source:      &source,
			hash:        uuid.NewString(),
			exists:      false,
		},
		{
			name:            "and save manifest returns error, then it returns same error",
			saveManifestErr: errTest,
			expectedErr:     errTest,
			source:          &source,
			hash:            uuid.NewString(),
			exists:          false,
		},
		{
			name:   "with valid data, then it returns a valid result",
			source: &source,
			hash:   uuid.NewString(),
			exists: false,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a Create service,
		when Do method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			sourceRepo := &snapshot.SourceRepositoryMock{}
			sourceRepo.ExploreFunc = func(_ context.Context, _ string) (*snapshot.Source, error) {
				return tt.source, tt.exploreErr
			}
			sourceRepo.CalculateHashFunc = func(_ context.Context, _ string) (string, error) {
				return tt.hash, tt.calculateHashErr
			}
			manifestRepo := &snapshot.ManifestRepositoryMock{}
			manifestRepo.SaveFunc = func(_ context.Context, _ *snapshot.Manifest) error {
				return tt.saveManifestErr
			}
			objRepo := &snapshot.ObjectRepositoryMock{}
			objRepo.SaveFunc = func(_ context.Context, _ *snapshot.File) error {
				return tt.saveObjErr
			}
			objRepo.AlreadyExistsFunc = func(_ context.Context, _ *snapshot.File) (bool, error) {
				return tt.exists, tt.alreadyExistsErr
			}
			folderRepositoryRep := &repository.FolderRepositoryMock{}
			folderRepositoryRep.GetConfigFunc = func(_ context.Context, _ string) (*repository.Config, error) {
				return nil, tt.getConfigErr
			}
			svc := snapshot.NewCreate(sourceRepo, manifestRepo, objRepo, repository.NewGetConfig(folderRepositoryRep))
			result, err := svc.Do(t.Context(), tt.args.repoPath, tt.args.sourcePath)
			if err != nil {
				require.ErrorAs(t, err, &tt.expectedErr)
				return
			}
			require.NotEmpty(t, result.Id())
			require.Equal(t, 1, result.FileCount())
			require.Equal(t, int64(20), result.TotalSize())
		})
	}
}
