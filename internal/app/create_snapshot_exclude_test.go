package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/stretchr/testify/require"
)

func TestCreateSnapshot_Handle_Exclude(t *testing.T) {
	t.Parallel()

	t.Run(`Given a source with an ignore file and command exclude patterns,
	when the CreateSnapshot command handler is called,
	then it explores the source with the ignore file patterns followed by the command patterns`, func(t *testing.T) {
		t.Parallel()

		deps := newCreateDependencies(t, sourceFiles(1))
		deps.sourceRepo.IgnorePatternsFunc = func(context.Context, string) ([]string, error) {
			return []string{"# logs", "*.log", "cache/"}, nil
		}
		explore := deps.sourceRepo.ExploreFunc
		var rules *snapshot.ExcludeRules
		deps.sourceRepo.ExploreFunc = func(ctx context.Context, sourcePath string, excludeRules *snapshot.ExcludeRules) (*snapshot.Source, error) {
			rules = excludeRules
			return explore(ctx, sourcePath, excludeRules)
		}
		command := createCommand
		command.Exclude = []string{"!keep.log", "/tmp"}

		_, err := createSnapshot(t.Context(), deps.handler(1), command)

		require.NoError(t, err)
		require.Equal(t, "source", deps.sourceRepo.IgnorePatternsCalls()[0].SourcePath)
		require.True(t, rules.Excludes("app/error.log", false))
		require.True(t, rules.Excludes("app/cache", true))
		require.True(t, rules.Excludes("tmp", true))
		require.False(t, rules.Excludes("keep.log", false), "command patterns must be applied after the ignore file")
		require.False(t, rules.Excludes("app/main.go", false))
	})

	errIgnore := errors.New("ignore file cannot be read")
	tests := []struct {
		name           string
		ignorePatterns []string
		ignoreErr      error
		exclude        []string
		expectedErr    error
	}{
		{
			name:        "and the ignore file cannot be read, then it returns the error",
			ignoreErr:   errIgnore,
			expectedErr: errIgnore,
		},
		{
			name:           "and the ignore file has an invalid pattern, then it returns an invalid exclude pattern error",
			ignorePatterns: []string{"[broken"},
			expectedErr:    snapshot.ErrInvalidExcludePattern,
		},
		{
			name:        "and an exclude pattern is invalid, then it returns an invalid exclude pattern error",
			exclude:     []string{"a//b"},
			expectedErr: snapshot.ErrInvalidExcludePattern,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a CreateSnapshot command handler,
		when Handle method is called `+tt.name+` without exploring the source`, func(t *testing.T) {
			t.Parallel()

			deps := newCreateDependencies(t, sourceFiles(1))
			deps.sourceRepo.IgnorePatternsFunc = func(context.Context, string) ([]string, error) {
				return tt.ignorePatterns, tt.ignoreErr
			}
			command := createCommand
			command.Exclude = tt.exclude

			_, err := createSnapshot(t.Context(), deps.handler(1), command)

			require.ErrorIs(t, err, tt.expectedErr)
			require.Empty(t, deps.sourceRepo.ExploreCalls())
			require.Empty(t, deps.manifestRepo.SaveCalls())
		})
	}
}
