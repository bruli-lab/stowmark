package snapshot_test

import (
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/stretchr/testify/require"
)

func TestNewRetentionPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		rules       snapshot.RetentionRules
		expectedErr error
	}{
		{
			name:        "without rules, then it returns an empty retention policy error",
			expectedErr: snapshot.ErrEmptyRetentionPolicy,
		},
		{
			name:        "with a negative count, then it returns a negative retention policy error",
			rules:       snapshot.RetentionRules{Last: 1, Daily: -1},
			expectedErr: snapshot.ErrNegativeRetentionPolicy,
		},
		{
			name:        "with a negative duration, then it returns a negative retention policy error",
			rules:       snapshot.RetentionRules{Within: -time.Hour},
			expectedErr: snapshot.ErrNegativeRetentionPolicy,
		},
		{
			name:  "with only a within duration, then it returns the policy",
			rules: snapshot.RetentionRules{Within: time.Hour},
		},
		{
			name:  "with a count, then it returns the policy",
			rules: snapshot.RetentionRules{Weekly: 2},
		},
	}
	for _, tt := range tests {
		t.Run(`Given retention rules,
		when NewRetentionPolicy is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			policy, err := snapshot.NewRetentionPolicy(tt.rules)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.rules, policy.Rules())
		})
	}
}

func resumeAt(id, source, createdAt string) snapshot.ManifestResume {
	created, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		panic(err)
	}
	return *snapshot.NewManifestResume(id, created, 1, 1, source)
}

func TestRetentionPolicy_Apply(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	snapshots := []snapshot.ManifestResume{
		resumeAt("d-older", "/home", "2026-10-01T03:00:00Z"),
		resumeAt("a-newest", "/home", "2026-11-14T10:00:00Z"),
		resumeAt("b-same-day", "/home", "2026-11-14T03:00:00Z"),
		resumeAt("c-yesterday", "/home", "2026-11-13T03:00:00Z"),
		resumeAt("e-last-week", "/home", "2026-11-06T03:00:00Z"),
		resumeAt("f-last-year", "/home", "2025-06-01T03:00:00Z"),
	}

	tests := []struct {
		name     string
		rules    snapshot.RetentionRules
		input    []snapshot.ManifestResume
		expected map[string][]snapshot.RetentionReason
	}{
		{
			name:  "with keep last, then it keeps the newest snapshots",
			rules: snapshot.RetentionRules{Last: 2},
			input: snapshots,
			expected: map[string][]snapshot.RetentionReason{
				"a-newest":   {snapshot.KeepLastReason},
				"b-same-day": {snapshot.KeepLastReason},
			},
		},
		{
			name:  "with keep daily, then it keeps the newest snapshot of each day",
			rules: snapshot.RetentionRules{Daily: 3},
			input: snapshots,
			expected: map[string][]snapshot.RetentionReason{
				"a-newest":    {snapshot.KeepDailyReason},
				"c-yesterday": {snapshot.KeepDailyReason},
				"e-last-week": {snapshot.KeepDailyReason},
			},
		},
		{
			name:  "with keep weekly and monthly, then it keeps the newest snapshot of each period",
			rules: snapshot.RetentionRules{Weekly: 2, Monthly: 3},
			input: snapshots,
			expected: map[string][]snapshot.RetentionReason{
				"a-newest":    {snapshot.KeepWeeklyReason, snapshot.KeepMonthlyReason},
				"e-last-week": {snapshot.KeepWeeklyReason},
				"d-older":     {snapshot.KeepMonthlyReason},
				"f-last-year": {snapshot.KeepMonthlyReason},
			},
		},
		{
			name:  "with keep yearly, then it keeps the newest snapshot of each year",
			rules: snapshot.RetentionRules{Yearly: 5},
			input: snapshots,
			expected: map[string][]snapshot.RetentionReason{
				"a-newest":    {snapshot.KeepYearlyReason},
				"f-last-year": {snapshot.KeepYearlyReason},
			},
		},
		{
			name:  "with keep within, then it keeps every snapshot newer than the duration",
			rules: snapshot.RetentionRules{Within: 48 * time.Hour},
			input: snapshots,
			expected: map[string][]snapshot.RetentionReason{
				"a-newest":    {snapshot.KeepWithinReason},
				"b-same-day":  {snapshot.KeepWithinReason},
				"c-yesterday": {snapshot.KeepWithinReason},
			},
		},
		{
			name:  "with snapshots of different sources, then it applies the rules to each source",
			rules: snapshot.RetentionRules{Last: 1},
			input: []snapshot.ManifestResume{
				resumeAt("home-old", "/home", "2026-11-01T03:00:00Z"),
				resumeAt("home-new", "/home", "2026-11-02T03:00:00Z"),
				resumeAt("etc-old", "/etc", "2026-11-01T03:00:00Z"),
			},
			expected: map[string][]snapshot.RetentionReason{
				"home-new": {snapshot.KeepLastReason},
				"etc-old":  {snapshot.KeepLastReason},
			},
		},
		{
			name:     "without snapshots, then it returns no decisions",
			rules:    snapshot.RetentionRules{Last: 1},
			expected: map[string][]snapshot.RetentionReason{},
		},
	}
	for _, tt := range tests {
		t.Run(`Given a retention policy,
		when Apply method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			policy, err := snapshot.NewRetentionPolicy(tt.rules)
			require.NoError(t, err)

			decisions := policy.Apply(tt.input, now)

			require.Len(t, decisions, len(tt.input))
			kept := map[string][]snapshot.RetentionReason{}
			for i, decision := range decisions {
				if i > 0 {
					require.False(t, decision.Snapshot().CreatedAt().After(decisions[i-1].Snapshot().CreatedAt()), "decisions must be ordered newest first")
				}
				if decision.Keep() {
					kept[decision.Snapshot().ID()] = decision.Reasons()
				}
			}
			require.Equal(t, tt.expected, kept)
		})
	}
}
