package cli

import (
	"encoding/json"
	"io/fs"
	"testing"
	"time"

	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestNewEntryDocument(t *testing.T) {
	t.Parallel()

	modifiedAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.FixedZone("CET", 3600))
	metadata := snapshot.NewMetadata(0o750|fs.ModeSetuid|fs.ModeSticky, modifiedAt)

	tests := []struct {
		name     string
		file     snapshot.File
		expected string
	}{
		{
			name:     "a regular file without metadata, then it has the hash and no mode",
			file:     fixtures.FileBuilder{Path: new("docs/a.txt"), Hash: new("aa01"), Size: new(int64(3))}.Build(),
			expected: `{"path":"docs/a.txt","kind":"file","hash":"aa01","size":3}`,
		},
		{
			name:     "a directory with metadata, then it has the unix mode in octal and the UTC modification time",
			file:     snapshot.NewDirectory("docs").WithMetadata(metadata),
			expected: `{"path":"docs","kind":"dir","size":0,"mode":"5750","modified_at":"2026-03-04T04:06:07Z"}`,
		},
		{
			name:     "a symlink, then it has the target",
			file:     *snapshot.NewSymlink("links/readme", "../README.txt"),
			expected: `{"path":"links/readme","kind":"symlink","size":0,"target":"../README.txt"}`,
		},
	}
	for _, tt := range tests {
		t.Run(`Given `+tt.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := json.Marshal(newEntryDocument(tt.file))

			require.NoError(t, err)
			require.JSONEq(t, tt.expected, string(encoded))
		})
	}
}

func TestDocuments_EmptyLists(t *testing.T) {
	t.Parallel()

	t.Run(`Given results without problems, snapshots or generations,
	when they are encoded as JSON,
	then every list is an empty array instead of null`, func(t *testing.T) {
		t.Parallel()

		documents := []any{
			newCheckDocument(snapshot.NewCheckReport(nil, nil, nil)),
			newPrunePlanDocument(snapshot.NewPrunePlan(nil, nil)),
			newPruneDocument(nil),
			newSnapshotDocuments(nil),
			newRetentionDocuments(nil),
			forgetDocument{Forgotten: newForgottenDocuments(nil), Postponed: newPostponedDocuments(nil)},
			newRetentionExtendDocument(nil),
			newResultDocument("s1", 0, nil, true),
		}
		for _, document := range documents {
			encoded, err := json.Marshal(document)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "null")
		}
	})
}

func TestNewPruneDocument(t *testing.T) {
	t.Parallel()

	t.Run(`Given the events of a prune that removed objects, a generation and temporaries and detected missing objects,
	when newPruneDocument is called,
	then it adds them up`, func(t *testing.T) {
		t.Parallel()

		manifests := []snapshot.Manifest{fixtures.ManifestBuilder{Files: []snapshot.File{
			fixtures.FileBuilder{Hash: new("aa01")}.Build(),
		}}.Build()}
		plan := snapshot.NewPrunePlan(manifests, []snapshot.StoredObject{
			snapshot.NewStoredObject("bb02", 10), snapshot.NewStoredObject("cc03", 32),
		})
		plan.RecordOrphanGenerations([]uint64{4})
		events := []event.Event{
			plan.MissingDetected("repo"),
			plan.Pruned("repo"),
			plan.GenerationsRemoved("repo"),
			snapshot.NewTemporariesRemoved("repo", 2),
		}

		document := newPruneDocument(events)

		require.Equal(t, &pruneDocument{
			RemovedObjects:          2,
			FreedBytes:              42,
			RemovedKeyGenerations:   []uint64{4},
			RemovedTemporaryFiles:   2,
			MissingObjects:          1,
			PostponedKeyGenerations: []uint64{},
		}, document)
	})

	t.Run(`Given the events of a prune where object lock postponed some deletions,
	when newPruneDocument is called,
	then it reports the postponed objects, bytes, generations and the end of the retention`, func(t *testing.T) {
		t.Parallel()

		plan := snapshot.NewPrunePlan(nil, []snapshot.StoredObject{snapshot.NewStoredObject("bb02", 10), snapshot.NewStoredObject("cc03", 32)})
		plan.RecordOrphanGenerations([]uint64{3, 4})
		until := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
		events := []event.Event{
			plan.PrunedExcept("repo", []string{"cc03"}),
			plan.GenerationsRemovedExcept("repo", []uint64{4}),
			plan.DeletionPostponed("repo", []string{"cc03"}, []uint64{4}, until),
		}

		document := newPruneDocument(events)

		require.Equal(t, &pruneDocument{
			RemovedObjects:          1,
			FreedBytes:              10,
			RemovedKeyGenerations:   []uint64{3},
			PostponedObjects:        1,
			PostponedBytes:          32,
			PostponedKeyGenerations: []uint64{4},
			RetainedUntil:           &until,
		}, document)
	})
}

func TestNewRetentionExtendDocument(t *testing.T) {
	t.Parallel()

	t.Run(`Given the event of an extended retention,
	when newRetentionExtendDocument is called,
	then it reports the extended items and the new end of the retention`, func(t *testing.T) {
		t.Parallel()

		conf := fixtures.ConfigBuilder{}.Build(t)
		until := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)

		document := newRetentionExtendDocument([]event.Event{conf.RetentionExtended(9, 4, until)})

		require.Equal(t, retentionExtendDocument{Extended: 4, RetainedUntil: &until}, document)
	})
}

func TestParseObjectLock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		period, mode string
		expected     time.Duration
		expectedErr  error
	}{
		{name: "without a period, then there is no object lock", mode: "governance"},
		{name: "with a period in days, then it returns the object lock", period: "30d", mode: "compliance", expected: 30 * 24 * time.Hour},
		{name: "with an invalid period, then it returns invalid duration error", period: "soon", mode: "governance", expectedErr: ErrInvalidRetentionDuration},
		{name: "with an unknown mode, then it returns invalid object lock error", period: "1d", mode: "legal", expectedErr: repository.ErrInvalidObjectLock},
		{name: "with a zero period, then it returns invalid object lock error", period: "0d", mode: "governance", expectedErr: repository.ErrInvalidObjectLock},
	}
	for _, tt := range tests {
		t.Run(`Given the object lock flags of init,
		when parseObjectLock is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			lock, err := parseObjectLock(tt.period, tt.mode)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			if tt.period == "" {
				require.Nil(t, lock)
				return
			}
			require.Equal(t, tt.expected, lock.Period())
			require.Equal(t, repository.ObjectLockMode(tt.mode), lock.Mode())
		})
	}
}
