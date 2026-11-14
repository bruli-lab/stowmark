package snapshot_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/stretchr/testify/require"
)

func TestNewExcludeRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		patterns []string
		valid    bool
	}{
		{name: "with comments, blank lines and valid patterns, then it returns the rules", patterns: []string{"# comment", "", "  ", "*.log", "/build/", "!keep.log", "docs/**/*.md"}, valid: true},
		{name: "with a malformed glob, then it returns an invalid exclude pattern error", patterns: []string{"[abc"}},
		{name: "with only a slash, then it returns an invalid exclude pattern error", patterns: []string{"/"}},
		{name: "with an empty segment, then it returns an invalid exclude pattern error", patterns: []string{"a//b"}},
		{name: "with only a negation, then it returns an invalid exclude pattern error", patterns: []string{"!"}},
	}
	for _, tt := range tests {
		t.Run(`Given exclude patterns,
		when NewExcludeRules is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			rules, err := snapshot.NewExcludeRules(tt.patterns)

			if !tt.valid {
				require.ErrorIs(t, err, snapshot.ErrInvalidExcludePattern)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, rules)
		})
	}
}

func TestExcludeRules_Excludes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		patterns []string
		path     string
		isDir    bool
		expected bool
	}{
		{name: "without patterns, then nothing is excluded", path: "a.txt"},
		{name: "with a basename glob, then it matches files at any depth", patterns: []string{"*.log"}, path: "var/app/error.log", expected: true},
		{name: "with a basename glob, then it does not match other extensions", patterns: []string{"*.log"}, path: "var/app/error.txt"},
		{name: "with a basename, then it matches directories at any depth", patterns: []string{"node_modules"}, path: "web/node_modules", isDir: true, expected: true},
		{name: "with a directory pattern, then it does not match a file with that name", patterns: []string{"cache/"}, path: "cache"},
		{name: "with a directory pattern, then it matches a directory with that name", patterns: []string{"cache/"}, path: "app/cache", isDir: true, expected: true},
		{name: "with an anchored pattern, then it matches only from the source root", patterns: []string{"/build"}, path: "build", isDir: true, expected: true},
		{name: "with an anchored pattern, then it does not match deeper paths", patterns: []string{"/build"}, path: "app/build", isDir: true},
		{name: "with a pattern containing a slash, then it is anchored to the source root", patterns: []string{"docs/*.md"}, path: "docs/readme.md", expected: true},
		{name: "with a pattern containing a slash, then it does not match the same path deeper", patterns: []string{"docs/*.md"}, path: "app/docs/readme.md"},
		{name: "with a double star in the middle, then it matches any number of directories", patterns: []string{"docs/**/*.md"}, path: "docs/a/b/readme.md", expected: true},
		{name: "with a double star in the middle, then it also matches no directories", patterns: []string{"docs/**/*.md"}, path: "docs/readme.md", expected: true},
		{name: "with a trailing double star, then it matches what is inside the directory", patterns: []string{"logs/**"}, path: "logs/today.txt", expected: true},
		{name: "with a trailing double star, then it does not match the directory itself", patterns: []string{"logs/**"}, path: "logs", isDir: true},
		{name: "with a negation after a match, then the last matching pattern wins", patterns: []string{"*.log", "!keep.log"}, path: "keep.log"},
		{name: "with a match after a negation, then the last matching pattern wins", patterns: []string{"!keep.log", "*.log"}, path: "keep.log", expected: true},
		{name: "with an escaped hash, then it matches a file starting with a hash", patterns: []string{`\#notes`}, path: "#notes", expected: true},
	}
	for _, tt := range tests {
		t.Run(`Given exclude rules,
		when Excludes method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			rules, err := snapshot.NewExcludeRules(tt.patterns)
			require.NoError(t, err)

			require.Equal(t, tt.expected, rules.Excludes(tt.path, tt.isDir))
		})
	}
}
