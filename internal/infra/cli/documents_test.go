package cli

import (
	"encoding/json"
	"io/fs"
	"testing"
	"time"

	"github.com/bruli-lab/go-core/event"
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
			forgetDocument{Forgotten: newForgottenDocuments(nil)},
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
			RemovedObjects:        2,
			FreedBytes:            42,
			RemovedKeyGenerations: []uint64{4},
			RemovedTemporaryFiles: 2,
			MissingObjects:        1,
		}, document)
	})
}
