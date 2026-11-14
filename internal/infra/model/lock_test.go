package model_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/stretchr/testify/require"
)

func TestLock_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run(`Given a domain lock,
	when it is serialized and hydrated again,
	then it keeps every field`, func(t *testing.T) {
		t.Parallel()

		lock, err := repository.NewLock(repository.ExclusiveLock, repository.NewLockOwner("host", 42), time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC))
		require.NoError(t, err)
		data, err := json.Marshal(model.NewLock(lock))
		require.NoError(t, err)

		got, err := model.BuildLockDomain(data, "locks/lock.json")

		require.NoError(t, err)
		require.Equal(t, lock, got)
	})
}

func TestBuildLockDomain_Invalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		data        string
		expectedErr error
	}{
		{name: "with invalid JSON, then it returns an error", data: "{"},
		{name: "with an invalid id, then it returns an error", data: `{"id":"x","kind":"shared","hostname":"h","created_at":"2026-11-14T10:00:00Z"}`},
		{
			name:        "with an unknown kind, then it returns an invalid lock kind error",
			data:        `{"id":"7f1c6f7e-8b8f-4c55-9d43-3c6a1e0b9a11","kind":"other","hostname":"h","created_at":"2026-11-14T10:00:00Z"}`,
			expectedErr: repository.ErrInvalidLockKind,
		},
		{
			name:        "without hostname, then it returns a missing lock owner error",
			data:        `{"id":"7f1c6f7e-8b8f-4c55-9d43-3c6a1e0b9a11","kind":"shared","created_at":"2026-11-14T10:00:00Z"}`,
			expectedErr: repository.ErrMissingLockOwner,
		},
	}
	for _, tt := range tests {
		t.Run(`Given lock data,
		when BuildLockDomain is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := model.BuildLockDomain([]byte(tt.data), "locks/lock.json")

			require.Error(t, err)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
			}
		})
	}
}
