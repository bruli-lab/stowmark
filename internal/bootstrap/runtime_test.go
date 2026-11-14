package bootstrap_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/bootstrap"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/stretchr/testify/require"
)

func newRuntime(t *testing.T) *bootstrap.Runtime {
	t.Helper()

	runtime, err := bootstrap.NewRuntime(context.Background(), map[string]string{"STOWMARK_LOG_LEVEL": "error"})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, runtime.Close(context.Background()))
	})
	return runtime
}

func TestNewRuntime(t *testing.T) {
	t.Parallel()

	t.Run(`Given an environment without observability endpoints,
	when NewRuntime is called,
	then every domain event has the log, trace and metrics listeners subscribed`, func(t *testing.T) {
		t.Parallel()

		runtime := newRuntime(t)

		for _, name := range bootstrap.DomainEvents {
			require.GreaterOrEqual(t, len(runtime.Listeners(name)), 3, name)
		}
		require.Len(t, runtime.Listeners(snapshot.SnapshotCreatedEventName), 4)
		require.Len(t, runtime.Listeners(snapshot.SnapshotRestoredEventName), 4)
	})

	t.Run(`Given an invalid observability configuration,
	when NewRuntime is called,
	then it returns the configuration error`, func(t *testing.T) {
		t.Parallel()

		_, err := bootstrap.NewRuntime(context.Background(), map[string]string{"OTEL_EXPORTER_OTLP_PROTOCOL": "carrier-pigeon", "OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:1"})

		require.Error(t, err)
	})
}

func TestRuntime_OpenRepository(t *testing.T) {
	t.Parallel()

	t.Run(`Given a runtime and a local repository path,
	when the repository is initialised and a snapshot is created through its handlers,
	then the handlers share the repository and return the domain events`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		source := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(source, "a.txt"), []byte("content"), 0o600))
		repo, err := newRuntime(t).OpenRepository(ctx, filepath.Join(t.TempDir(), "repo"))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, repo.Close()) })
		comp, err := repository.NewCompression(repository.NoneCompressionType, nil)
		require.NoError(t, err)

		events, err := repo.Init().Handle(ctx, app.InitCommand{RepositoryPath: repo.Path(), Compression: comp, FormatVersion: repository.CurrentFormatVersion})
		require.NoError(t, err)
		_, err = app.SingleEvent[*repository.RepositoryInitialized](events)
		require.NoError(t, err)

		events, err = repo.CreateSnapshot().Handle(ctx, app.CreateSnapshotCommand{RepositoryPath: repo.Path(), SourcePath: source})
		require.NoError(t, err)
		created, err := app.SingleEvent[*snapshot.SnapshotCreated](events)
		require.NoError(t, err)
		require.Equal(t, 1, created.FileCount())

		snapshots, err := app.QueryResult[[]snapshot.ManifestResume](repo.SnapshotList().Handle(ctx, app.SnapshotListQuery{}))
		require.NoError(t, err)
		require.Len(t, snapshots, 1)
		require.Equal(t, created.SnapshotID(), snapshots[0].ID())
	})

	t.Run(`Given a runtime and an SSH repository without the SSH key in the environment,
	when OpenRepository is called,
	then it returns the configuration error`, func(t *testing.T) {
		t.Parallel()

		_, err := newRuntime(t).OpenRepository(context.Background(), "ssh://user@localhost:22/backups")

		require.Error(t, err)
	})
}
