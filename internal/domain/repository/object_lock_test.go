package repository_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/stretchr/testify/require"
)

func TestNewObjectLock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mode        repository.ObjectLockMode
		period      time.Duration
		expected    time.Duration
		expectedErr error
	}{
		{name: "with governance mode and a period, then it truncates the period to seconds", mode: repository.GovernanceObjectLock, period: 90*time.Second + 500*time.Millisecond, expected: 90 * time.Second},
		{name: "with compliance mode, then it accepts it", mode: repository.ComplianceObjectLock, period: time.Hour, expected: time.Hour},
		{name: "with an unknown mode, then it returns invalid object lock error", mode: "legal", period: time.Hour, expectedErr: repository.ErrInvalidObjectLock},
		{name: "with a period under one second, then it returns invalid object lock error", mode: repository.GovernanceObjectLock, period: time.Millisecond, expectedErr: repository.ErrInvalidObjectLock},
	}
	for _, tt := range tests {
		t.Run(`Given an object lock mode and period,
		when NewObjectLock is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			lock, err := repository.NewObjectLock(tt.mode, tt.period)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.mode, lock.Mode())
			require.Equal(t, tt.expected, lock.Period())
		})
	}
}

func TestParseObjectLockMode(t *testing.T) {
	t.Parallel()

	t.Run(`Given the names of the object lock modes,
	when ParseObjectLockMode is called,
	then it accepts governance and compliance only`, func(t *testing.T) {
		t.Parallel()

		governance, err := repository.ParseObjectLockMode("governance")
		require.NoError(t, err)
		require.Equal(t, repository.GovernanceObjectLock, governance)
		compliance, err := repository.ParseObjectLockMode("compliance")
		require.NoError(t, err)
		require.Equal(t, repository.ComplianceObjectLock, compliance)
		_, err = repository.ParseObjectLockMode("GOVERNANCE")
		require.ErrorIs(t, err, repository.ErrInvalidObjectLock)
	})
}

func TestObjectLock_Retention(t *testing.T) {
	t.Parallel()

	lock, err := repository.NewObjectLock(repository.ComplianceObjectLock, 10*24*time.Hour)
	require.NoError(t, err)
	now := time.Date(2026, time.November, 14, 10, 30, 15, 700, time.FixedZone("CET", 3600))

	t.Run(`Given an object lock,
	when RetentionFrom is called,
	then the retention ends one period later, in UTC and truncated to seconds`, func(t *testing.T) {
		t.Parallel()

		retention := lock.RetentionFrom(now)

		require.Equal(t, repository.ComplianceObjectLock, retention.Mode())
		require.Equal(t, time.Date(2026, time.November, 24, 9, 30, 15, 0, time.UTC), retention.Until())
	})

	tests := []struct {
		name     string
		until    time.Time
		expected bool
	}{
		{name: "already expired, then it needs an extension", until: now.Add(-time.Hour), expected: true},
		{name: "ending within half a period, then it needs an extension", until: now.Add(4 * 24 * time.Hour), expected: true},
		{name: "ending after half a period, then it does not need an extension", until: now.Add(6 * 24 * time.Hour), expected: false},
	}
	for _, tt := range tests {
		t.Run(`Given an object lock and a retention `+tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expected, lock.NeedsExtension(tt.until, now))
		})
	}
}

func TestRetainedError(t *testing.T) {
	t.Parallel()

	t.Run(`Given a retained error wrapped by another error,
	when RetainedUntil is called,
	then it returns the end of the retention`, func(t *testing.T) {
		t.Parallel()

		until := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
		err := fmt.Errorf("delete object: %w", repository.NewRetainedError(until))

		got, retained := repository.RetainedUntil(err)

		require.True(t, retained)
		require.Equal(t, until, got)
		require.ErrorIs(t, err, repository.ErrRetained)
		require.Contains(t, err.Error(), "2026-12-01T00:00:00Z")
	})

	t.Run(`Given errors that are not retentions,
	when RetainedUntil is called,
	then it reports that nothing is retained`, func(t *testing.T) {
		t.Parallel()

		_, retained := repository.RetainedUntil(errors.New("boom"))
		require.False(t, retained)
		_, retained = repository.RetainedUntil(nil)
		require.False(t, retained)
		until, retained := repository.RetainedUntil(repository.ErrRetained)
		require.True(t, retained)
		require.True(t, until.IsZero())
		require.Equal(t, repository.ErrRetained.Error(), repository.NewRetainedError(time.Time{}).Error())
	})
}

func TestRetainedItem(t *testing.T) {
	t.Parallel()

	t.Run(`Given the retained item constructors,
	when they are called,
	then each item has its kind and id`, func(t *testing.T) {
		t.Parallel()

		object := repository.NewRetainedObject("aa01", true, 3)
		require.Equal(t, repository.RetainedObject, object.Kind())
		require.Equal(t, "aa01", object.ID())
		require.True(t, object.Encrypted())
		require.Equal(t, uint64(3), object.Generation())
		require.Equal(t, repository.RetainedManifest, repository.NewRetainedManifest("s1").Kind())
		require.Equal(t, "s1", repository.NewRetainedManifest("s1").ID())
		require.Equal(t, repository.RetainedConfig, repository.NewRetainedConfig().Kind())
		require.Empty(t, repository.NewRetainedConfig().ID())
	})
}
