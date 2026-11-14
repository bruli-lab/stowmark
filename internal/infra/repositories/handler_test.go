package repositories_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
	"github.com/stretchr/testify/require"
)

func requireRepositories(t *testing.T, repos repositories.Repositories, expectedPath string) {
	t.Helper()

	require.Equal(t, expectedPath, repos.RepositoryPath())
	require.NotNil(t, repos.FolderRepository())
	store, err := repos.BlobStore()
	require.NoError(t, err)
	require.NotNil(t, store)
	objects, err := repos.ObjectRepository()
	require.NoError(t, err)
	require.NotNil(t, objects)
	encrypted, err := repos.EncryptedObjectRepository()
	require.NoError(t, err)
	require.NotNil(t, encrypted)
	manifests, err := repos.ManifestRepository()
	require.NoError(t, err)
	require.NotNil(t, manifests)
	locks, err := repos.LockRepository()
	require.NoError(t, err)
	require.NotNil(t, locks)
	retention, err := repos.RetentionRepository()
	require.NoError(t, err)
	require.NotNil(t, retention)
}

func TestNewHandler(t *testing.T) {
	t.Parallel()

	t.Run(`Given a local path,
	when NewHandler is called,
	then it returns every repository of the local backend, without object lock`, func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "repo")

		repos, err := repositories.NewHandler(t.Context(), path, nil)

		require.NoError(t, err)
		requireRepositories(t, repos, path)
		retention, err := repos.RetentionRepository()
		require.NoError(t, err)
		require.ErrorIs(t, retention.VerifyObjectLock(t.Context()), repository.ErrObjectLockUnsupported)
		require.NoError(t, repos.Close())
	})

	t.Run(`Given an S3 URL and the S3 configuration,
	when NewHandler is called,
	then it returns the repositories for the prefix of the bucket without connecting`, func(t *testing.T) {
		t.Parallel()

		repos, err := repositories.NewHandler(t.Context(), "s3://backups/home/server", map[string]string{
			"AWS_REGION":             "eu-west-1",
			"AWS_ACCESS_KEY_ID":      "key",
			"AWS_SECRET_ACCESS_KEY":  "secret",
			"STOWMARK_S3_ENDPOINT":   "http://127.0.0.1:1",
			"STOWMARK_S3_PATH_STYLE": "true",
		})

		require.NoError(t, err)
		requireRepositories(t, repos, "home/server")
		require.NoError(t, repos.Close())
	})

	t.Run(`Given a GCS URL and an emulator endpoint,
	when NewHandler is called,
	then it returns the repositories of the bucket`, func(t *testing.T) {
		t.Parallel()

		repos, err := repositories.NewHandler(t.Context(), "gcs://backups/home", map[string]string{
			"STOWMARK_GCS_ENDPOINT": "http://127.0.0.1:1/storage/v1/",
		})

		require.NoError(t, err)
		requireRepositories(t, repos, "gcs://backups/home")
		require.NoError(t, repos.Close())
	})

	t.Run(`Given a WebDAV URL of a reachable server and its credentials,
	when NewHandler is called,
	then it connects and returns the repositories of the path`, func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

		repos, err := repositories.NewHandler(t.Context(), server.URL+"/backups", map[string]string{
			"STOWMARK_WEBDAV_USERNAME": "user",
			"STOWMARK_WEBDAV_PASSWORD": "password",
		})

		require.NoError(t, err)
		requireRepositories(t, repos, "backups")
		require.NoError(t, repos.Close())
	})
}

func TestNewHandler_Errors(t *testing.T) {
	t.Parallel()

	closed := closedAddress(t)

	tests := []struct {
		name        string
		url         string
		environment map[string]string
		expectedErr error
		contains    string
	}{
		{name: "with an empty path, then it returns invalid repository path error", url: "", expectedErr: repository.ErrInvalidRepositoryPath},
		{name: "with an SSH URL without user, then it returns a username error", url: "ssh://example.com/backups", environment: map[string]string{"STOWMARK_SSH_PRIVATE_KEY": "key"}, contains: "SSH username is required"},
		{name: "with an SSH URL without host, then it returns a host error", url: "ssh://user@/backups", environment: map[string]string{"STOWMARK_SSH_PRIVATE_KEY": "key"}, contains: "SSH host is required"},
		{name: "with an SMB URL without the password, then it returns a configuration error", url: "smb://user@" + closed + "/share/backups", contains: "SMB_PASSWORD"},
		{name: "with an SMB server that is not listening, then it returns the connection error", url: "smb://user@" + closed + "/share/backups", environment: map[string]string{"STOWMARK_SMB_PASSWORD": "secret"}, contains: closed},
		{name: "with a WebDAV URL without credentials, then it returns a configuration error", url: "https://" + closed + "/backups", contains: "WEBDAV_USERNAME"},
		{name: "with a WebDAV server that is not listening, then it returns the connection error", url: "http://" + closed + "/backups", environment: map[string]string{"STOWMARK_WEBDAV_USERNAME": "user", "STOWMARK_WEBDAV_PASSWORD": "password"}, contains: "connect to WebDAV server"},
		{name: "with an S3 URL without bucket, then it returns an error", url: "s3:///backups", environment: map[string]string{"AWS_REGION": "eu-west-1"}, contains: "bucket"},
	}
	for _, tt := range tests {
		t.Run(`Given a repository URL,
		when NewHandler is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			environment := tt.environment
			if environment == nil {
				environment = map[string]string{}
			}

			repos, err := repositories.NewHandler(context.Background(), tt.url, environment)

			require.Error(t, err)
			require.Nil(t, repos)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
			}
			if tt.contains != "" {
				require.ErrorContains(t, err, tt.contains)
			}
		})
	}
}

func closedAddress(t *testing.T) string {
	t.Helper()
	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	return address
}
