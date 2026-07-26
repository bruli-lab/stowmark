package repository_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/stretchr/testify/require"
)

func TestParseRepository(t *testing.T) {
	type args struct {
		raw string
	}
	tests := []struct {
		name         string
		args         args
		expectedType repository.RepositoryType
		expectedErr  error
	}{
		{
			name:        "with an empty repository path, then it should return a invalid repository path error",
			expectedErr: repository.ErrInvalidRepositoryPath,
		},
		{
			name:         "with a local path, then it should return a local repository type",
			args:         args{raw: "tmp/test"},
			expectedType: repository.RepositoryLocal,
		},
		{
			name:         "with a ssh path, then it should return a ssh repository type",
			args:         args{raw: "ssh://user@host:/tmp/test"},
			expectedType: repository.RepositorySSH,
		},
		{
			name:         "with a smb path, then it should return a smb repository type",
			args:         args{raw: "smb://host/tmp/test"},
			expectedType: repository.RepositorySMB,
		},
		{
			name:         "with a s3 path, then it should return a s3 repository type",
			args:         args{raw: "s3://bucket/tmp/test"},
			expectedType: repository.RepositoryS3,
		},
		{
			name:         "with a webdav path, then it should return a webdav repository type",
			args:         args{raw: "webdav://host/tmp/test"},
			expectedType: repository.RepositoryWebDAV,
		},
		{
			name:         "with a webdavs path, then it should return a webdav repository type",
			args:         args{raw: "webdavs://host/tmp/test"},
			expectedType: repository.RepositoryWebDAV,
		},
		{
			name:         "with a https path, then it should return a webdav repository type",
			args:         args{raw: "https://host/tmp/test"},
			expectedType: repository.RepositoryWebDAV,
		},
		{
			name:         "with a http path, then it should return a webdav repository type",
			args:         args{raw: "http://host/tmp/test"},
			expectedType: repository.RepositoryWebDAV,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a ParseRepositoryType method 
		when is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := repository.ParseRepositoryType(tt.args.raw)
			if err != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.Equal(t, tt.expectedType, got)
		})
	}
}
