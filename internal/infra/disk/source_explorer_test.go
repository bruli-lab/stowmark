package disk_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func explored(t *testing.T, root string, kind snapshot.EntryKind, patterns ...string) []string {
	t.Helper()
	rules, err := snapshot.NewExcludeRules(patterns)
	require.NoError(t, err)

	source, err := disk.NewSourceRepository().Explore(context.Background(), root, rules)
	require.NoError(t, err)

	paths := make([]string, 0, len(source.Files()))
	for _, file := range source.Files() {
		if file.Kind() != kind {
			continue
		}
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
		name        string
		patterns    []string
		expected    []string
		directories []string
	}{
		{
			name:        "without patterns, then it returns every regular file and directory",
			expected:    files,
			directories: []string{"web", "web/node_modules", "web/node_modules/lib", "logs", "build", "app", "app/build"},
		},
		{
			name:        "with basename, directory and anchored patterns, then it skips the excluded files and directories",
			patterns:    []string{"*.log", "node_modules/", "/build"},
			expected:    []string{".stowmarkignore", "main.go", "web/app.js", "logs/keep.txt", "app/build/out.bin"},
			directories: []string{"web", "logs", "app", "app/build"},
		},
		{
			name:        "with a negation inside an excluded directory, then the directory stays excluded",
			patterns:    []string{"logs/", "!logs/keep.txt"},
			expected:    []string{".stowmarkignore", "main.go", "debug.log", "web/node_modules/lib/index.js", "web/app.js", "build/out.bin", "app/build/out.bin"},
			directories: []string{"web", "web/node_modules", "web/node_modules/lib", "build", "app", "app/build"},
		},
		{
			name:        "with a trailing double star and a negation, then the negated file is kept",
			patterns:    []string{"logs/**", "!logs/keep.txt"},
			expected:    []string{".stowmarkignore", "main.go", "debug.log", "web/node_modules/lib/index.js", "web/app.js", "logs/keep.txt", "build/out.bin", "app/build/out.bin"},
			directories: []string{"web", "web/node_modules", "web/node_modules/lib", "logs", "build", "app", "app/build"},
		},
	}
	for _, tt := range tests {
		t.Run(`Given a source directory,
		when Explore is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeTree(t, root, files...)

			require.ElementsMatch(t, tt.expected, explored(t, root, snapshot.RegularEntry, tt.patterns...))
			require.ElementsMatch(t, tt.directories, explored(t, root, snapshot.DirectoryEntry, tt.patterns...))
		})
	}
}

func TestSourceExplorer_Explore_Metadata(t *testing.T) {
	t.Parallel()

	t.Run(`Given a source file with permissions and a modification time,
	when Explore is called,
	then the entry carries them`, func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeTree(t, root, "bin/run.sh")
		path := filepath.Join(root, "bin", "run.sh")
		modTime := time.Date(2024, 3, 4, 5, 6, 7, 0, time.UTC)
		require.NoError(t, os.Chmod(path, 0o750))
		require.NoError(t, os.Chtimes(path, modTime, modTime))

		source, err := disk.NewSourceRepository().Explore(context.Background(), root, nil)

		require.NoError(t, err)
		var found bool
		for _, file := range source.Files() {
			if file.Path() != path {
				continue
			}
			found = true
			metadata, ok := file.Metadata()
			require.True(t, ok)
			require.Equal(t, os.FileMode(0o750), metadata.Mode())
			require.True(t, modTime.Equal(metadata.ModTime()))
		}
		require.True(t, found)
	})
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

func TestSourceExplorer_Explore_NestedIgnoreFiles(t *testing.T) {
	t.Parallel()

	files := []string{
		"main.log",
		"app/debug.log",
		"app/keep.log",
		"app/build/out.bin",
		"app/lib/build/out.bin",
		"web/build/out.bin",
	}
	tests := []struct {
		name     string
		skip     bool
		override []string
		expected []string
	}{
		{
			name:     "then each ignore file applies to its own directory and can override its parents",
			expected: []string{".stowmarkignore", "app/.stowmarkignore", "app/keep.log", "app/lib/build/out.bin", "web/build/out.bin"},
		},
		{
			name:     "with an exclude override, then it wins over the nested ignore files",
			override: []string{"keep.log"},
			expected: []string{".stowmarkignore", "app/.stowmarkignore", "app/lib/build/out.bin", "web/build/out.bin"},
		},
		{
			name:     "without reading ignore files, then only the command patterns apply",
			skip:     true,
			override: []string{"web/"},
			expected: append([]string{".stowmarkignore", "app/.stowmarkignore"}, files[:5]...),
		},
	}
	for _, tt := range tests {
		t.Run(`Given a source with a root ignore file and a nested ignore file,
		when Explore is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeTree(t, root, files...)
			require.NoError(t, os.WriteFile(filepath.Join(root, snapshot.IgnoreFileName), []byte("*.log\n"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "app", snapshot.IgnoreFileName), []byte("!keep.log\n/build/\n"), 0o600))
			explorer := disk.NewSourceRepository()
			var rootPatterns []string
			if !tt.skip {
				patterns, err := explorer.IgnorePatterns(context.Background(), root)
				require.NoError(t, err)
				rootPatterns = patterns
			}
			rules, err := snapshot.NewExcludeRules(rootPatterns)
			require.NoError(t, err)
			rules, err = rules.WithOverrides(tt.override)
			require.NoError(t, err)
			if tt.skip {
				rules = rules.WithoutIgnoreFiles()
			}

			source, err := explorer.Explore(context.Background(), root, rules)

			require.NoError(t, err)
			paths := make([]string, 0, len(source.Files()))
			for _, file := range source.Files() {
				if file.Kind() != snapshot.RegularEntry {
					continue
				}
				relative, err := filepath.Rel(root, file.Path())
				require.NoError(t, err)
				paths = append(paths, filepath.ToSlash(relative))
			}
			require.ElementsMatch(t, tt.expected, paths)
		})
	}

	t.Run(`Given a source with a nested ignore file with an invalid pattern,
	when Explore is called,
	then it returns an invalid exclude pattern error`, func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeTree(t, root, "app/main.go")
		require.NoError(t, os.WriteFile(filepath.Join(root, "app", snapshot.IgnoreFileName), []byte("[broken\n"), 0o600))
		rules, err := snapshot.NewExcludeRules(nil)
		require.NoError(t, err)

		_, err = disk.NewSourceRepository().Explore(context.Background(), root, rules)

		require.ErrorIs(t, err, snapshot.ErrInvalidExcludePattern)
	})
}
