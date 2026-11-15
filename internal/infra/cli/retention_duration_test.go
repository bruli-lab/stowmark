package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseRetentionDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		value       string
		expected    time.Duration
		expectedErr error
	}{
		{name: "with an empty value, then it returns zero"},
		{name: "with days, then it returns the days", value: "30d", expected: 30 * 24 * time.Hour},
		{name: "with weeks, then it returns the weeks", value: "2w", expected: 14 * 24 * time.Hour},
		{name: "with a Go duration, then it returns the duration", value: "36h", expected: 36 * time.Hour},
		{name: "with a non numeric day count, then it returns an error", value: "xd", expectedErr: ErrInvalidRetentionDuration},
		{name: "with a negative day count, then it returns an error", value: "-1d", expectedErr: ErrInvalidRetentionDuration},
		{name: "with an unknown unit, then it returns an error", value: "3y", expectedErr: ErrInvalidRetentionDuration},
	}
	for _, tt := range tests {
		t.Run(`Given a keep-within value,
		when parseRetentionDuration is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			duration, err := parseRetentionDuration(tt.value)

			require.ErrorIs(t, err, tt.expectedErr)
			require.Equal(t, tt.expected, duration)
		})
	}
}
