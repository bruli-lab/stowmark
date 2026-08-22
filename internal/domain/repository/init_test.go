package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestInit_Do(t *testing.T) {
	errTest := errors.New("test error")
	re := fixtures.RepositoryBuilder{}.Build(t)
	type args struct {
		r *repository.Repository
	}
	tests := []struct {
		name string
		args args
		expectedErr, existErr,
		createRepoErr, createConfigErr,
		createObjectErr, createSnapshotErr,
		getConfigErr error
		exists         bool
		previousConfig *repository.Config
	}{
		{
			name:        "and exists returns an error, then it returns same error",
			existErr:    errTest,
			args:        args{r: &re},
			expectedErr: errTest,
		},
		{
			name:          "and create repository folder returns an error, then it returns same error",
			exists:        false,
			args:          args{r: &re},
			createRepoErr: errTest,
			expectedErr:   errTest,
		},
		{
			name:           "and get config returns an error, then it returns same error",
			exists:         true,
			args:           args{r: &re},
			previousConfig: &repository.Config{},
			getConfigErr:   errTest,
			expectedErr:    errTest,
		},
		{
			name:            "and create config returns an error, then it returns same error",
			exists:          true,
			args:            args{r: &re},
			previousConfig:  &repository.Config{},
			createConfigErr: errTest,
			expectedErr:     errTest,
		},
		{
			name:            "and create objects folder returns an error, then it returns same error",
			exists:          true,
			args:            args{r: &re},
			previousConfig:  &repository.Config{},
			createObjectErr: errTest,
			expectedErr:     errTest,
		},
		{
			name:              "and create snapshots folder returns an error, then it returns same error",
			exists:            true,
			args:              args{r: &re},
			previousConfig:    &repository.Config{},
			createSnapshotErr: errTest,
			expectedErr:       errTest,
		},
		{
			name:   "and all works fine, then it returns nil",
			exists: false,
			args:   args{r: &re},
		},
	}
	for _, tt := range tests {
		t.Run(`Given a Init struct,
		when Do method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &repository.FolderRepositoryMock{
				ExistsFunc: func(_ context.Context, _ string) (bool, error) {
					return tt.exists, tt.existErr
				},
				CreateConfigFunc: func(_ context.Context, _ string, _ *repository.Config) error {
					return tt.createConfigErr
				},
				GetConfigFunc: func(_ context.Context, _ string) (*repository.Config, error) {
					return tt.previousConfig, tt.getConfigErr
				},
			}
			repo.CreateFolderFunc = func(_ context.Context, _ string) error {
				call := len(repo.CreateFolderCalls())
				switch call {
				case 1:
					return tt.createRepoErr
				case 2:
					return tt.createObjectErr
				case 3:
					return tt.createSnapshotErr
				}
				return nil
			}
			svc := repository.NewInit(repo)
			_, err := svc.Do(t.Context(), tt.args.r, false)
			if err != nil {
				require.ErrorAs(t, err, &tt.expectedErr)
				return
			}
			if tt.expectedErr != nil {
				require.Error(t, err)
			}
		})
	}
}
