package disk_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/stretchr/testify/require"
)

func writeTree(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, file := range files {
		path := filepath.Join(root, filepath.FromSlash(file))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(file), 0o600))
	}
}

func explored(t *testing.T, root string, patterns ...string) []string {
	t.Helper()
	rules, err := snapshot.NewExcludeRules(patterns)
	require.NoError(t, err)

	source, err := disk.NewSourceRepository().Explore(context.Background(), root, rules)
	require.NoError(t, err)

	paths := make([]string, 0, len(source.Files()))
	for _, file := range source.Files() {
		relative, err := filepath.Rel(source.AbsolutePath(), file.Path())
		require.NoError(t, err)
		paths = append(paths, filepath.ToSlash(relative))
	}
	return paths
}

func TestSourceExplorer_Explore(t *testing.T) {
	t.Parallel()

	files := []string{
		".stowmarkignore",
		"main.go",
		"debug.log",
		"web/node_modules/lib/index.js",
		"web/app.js",
		"logs/today.log",
		"logs/keep.txt",
		"build/out.bin",
		"app/build/out.bin",
	}

	tests := []struct {
		name     string
		patterns []string
		expected []string
	}{
		{
			name:     "without patterns, then it returns every regular file",
			expected: files,
		},
		{
			name:     "with basename, directory and anchored patterns, then it skips the excluded files and directories",
			patterns: []string{"*.log", "node_modules/", "/build"},
			expected: []string{".stowmarkignore", "main.go", "web/app.js", "logs/keep.txt", "app/build/out.bin"},
		},
		{
			name:     "with a negation inside an excluded directory, then the directory stays excluded",
			patterns: []string{"logs/", "!logs/keep.txt"},
			expected: []string{".stowmarkignore", "main.go", "debug.log", "web/node_modules/lib/index.js", "web/app.js", "build/out.bin", "app/build/out.bin"},
		},
		{
			name:     "with a trailing double star and a negation, then the negated file is kept",
			patterns: []string{"logs/**", "!logs/keep.txt"},
			expected: []string{".stowmarkignore", "main.go", "debug.log", "web/node_modules/lib/index.js", "web/app.js", "logs/keep.txt", "build/out.bin", "app/build/out.bin"},
		},
	}
	for _, tt := range tests {
		t.Run(`Given a source directory,
		when Explore is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeTree(t, root, files...)

			require.ElementsMatch(t, tt.expected, explored(t, root, tt.patterns...))
		})
	}
}

func TestSourceExplorer_IgnorePatterns(t *testing.T) {
	t.Parallel()

	t.Run(`Given a source directory with an ignore file,
	when IgnorePatterns is called,
	then it returns its lines`, func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, snapshot.IgnoreFileName), []byte("# comment\n*.log\r\nnode_modules/\n"), 0o600))

		patterns, err := disk.NewSourceRepository().IgnorePatterns(context.Background(), root)

		require.NoError(t, err)
		require.Equal(t, []string{"# comment", "*.log\r", "node_modules/", ""}, patterns)
		rules, err := snapshot.NewExcludeRules(patterns)
		require.NoError(t, err)
		require.True(t, rules.Excludes("a.log", false))
	})

	t.Run(`Given a source directory without an ignore file,
	when IgnorePatterns is called,
	then it returns no patterns`, func(t *testing.T) {
		t.Parallel()

		patterns, err := disk.NewSourceRepository().IgnorePatterns(context.Background(), t.TempDir())

		require.NoError(t, err)
		require.Empty(t, patterns)
	})

	t.Run(`Given a source whose ignore file is a directory,
	when IgnorePatterns is called,
	then it returns an error`, func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, snapshot.IgnoreFileName), 0o755))

		_, err := disk.NewSourceRepository().IgnorePatterns(context.Background(), root)

		require.Error(t, err)
	})
}
