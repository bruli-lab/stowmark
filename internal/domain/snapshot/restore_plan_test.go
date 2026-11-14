package snapshot_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestNewRestorePlan(t *testing.T) {
	t.Parallel()

	t.Run(`Given the entries of a manifest,
	when NewRestorePlan is called,
	then it creates the directories parents first, then the files, and the symlinks last so no file is written through a restored link`, func(t *testing.T) {
		t.Parallel()

		file := fixtures.FileBuilder{Path: new("/data/docs/file.txt")}.Build()
		link := *snapshot.NewSymlink("/data/link", "docs")
		docs := *snapshot.NewDirectory("/data/docs")
		nested := *snapshot.NewDirectory("/data/docs/nested")

		plan := snapshot.NewRestorePlan([]snapshot.File{link, nested, file, docs})

		require.Equal(t, []snapshot.File{docs, nested}, plan.Directories())
		require.Equal(t, []snapshot.File{file}, plan.Files())
		require.Equal(t, []snapshot.File{link}, plan.Symlinks())
		require.Equal(t, []snapshot.File{nested, docs}, plan.DirectoriesDeepestFirst())
		require.Equal(t, []snapshot.File{docs, nested}, plan.Directories(), "the plan never changes")
	})
}
