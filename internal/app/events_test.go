package app_test

import (
	"testing"

	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/stretchr/testify/require"
)

func TestEventsOf(t *testing.T) {
	t.Parallel()

	first := snapshot.NewFileRestored("snapshot-id", "a")
	second := snapshot.NewFileRestored("snapshot-id", "b")
	created := createdEvent()
	events := []event.Event{first, created, second}

	t.Run(`Given events of several types,
	when EventsOf is called with one type,
	then it returns only the events of that type in order`, func(t *testing.T) {
		t.Parallel()

		require.Equal(t, []*snapshot.FileRestored{first, second}, app.EventsOf[*snapshot.FileRestored](events))
		require.Empty(t, app.EventsOf[*snapshot.SnapshotForgotten](events))
	})

	t.Run(`Given events of several types,
	when SingleEvent is called,
	then it returns the only event of that type or an error when there is not exactly one`, func(t *testing.T) {
		t.Parallel()

		got, err := app.SingleEvent[*snapshot.SnapshotCreated](events)
		require.NoError(t, err)
		require.Equal(t, created, got)

		_, err = app.SingleEvent[*snapshot.FileRestored](events)
		require.Error(t, err)
		_, err = app.SingleEvent[*snapshot.SnapshotForgotten](events)
		require.Error(t, err)
	})
}
