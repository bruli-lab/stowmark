package snapshot

import (
	"time"

	"github.com/bruli-lab/go-core/event"
	"github.com/google/uuid"
)

const (
	SnapshotCreatedEventName        = "snapshot.created"
	SnapshotRestoredEventName       = "snapshot.restored"
	FileRestoredEventName           = "snapshot.file_restored"
	SnapshotForgottenEventName      = "snapshot.forgotten"
	ObjectsPrunedEventName          = "objects.pruned"
	MissingObjectsDetectedEventName = "objects.missing_detected"
	GenerationsRemovedEventName     = "objects.generations_removed"
	TemporariesRemovedEventName     = "objects.temporaries_removed"
)

type SnapshotCreated struct {
	event.BasicEvent
	fileCount int
	totalSize int64
	skipped   []SkippedFile
}

func (s *SnapshotCreated) SnapshotID() string {
	return s.AggregateRootID()
}

func (s *SnapshotCreated) FileCount() int {
	return s.fileCount
}

func (s *SnapshotCreated) TotalSize() int64 {
	return s.totalSize
}

func (s *SnapshotCreated) SkippedFiles() []SkippedFile {
	return s.skipped
}

func NewSnapshotCreated(manifest *Manifest, savedFiles int, savedSize int64, skipped []SkippedFile) *SnapshotCreated {
	return &SnapshotCreated{
		BasicEvent: event.NewBasicEvent(SnapshotCreatedEventName, uuid.New(), manifest.ID()),
		fileCount:  savedFiles,
		totalSize:  savedSize,
		skipped:    skipped,
	}
}

type SnapshotRestored struct {
	event.BasicEvent
	totalFiles int
	failed     []FailedResult
}

func (s *SnapshotRestored) SnapshotID() string {
	return s.AggregateRootID()
}

func (s *SnapshotRestored) TotalFiles() int {
	return s.totalFiles
}

func (s *SnapshotRestored) Failed() []FailedResult {
	return s.failed
}

func (s *SnapshotRestored) IsSuccess() bool {
	return len(s.failed) == 0
}

func (v *Result) Restored() *SnapshotRestored {
	return &SnapshotRestored{
		BasicEvent: event.NewBasicEvent(SnapshotRestoredEventName, uuid.New(), v.snapshotID),
		totalFiles: v.totalFiles,
		failed:     v.failed,
	}
}

type FileRestored struct {
	event.BasicEvent
	filePath string
}

func (f *FileRestored) SnapshotID() string {
	return f.AggregateRootID()
}

func (f *FileRestored) FilePath() string {
	return f.filePath
}

func NewFileRestored(snapshotID, filePath string) *FileRestored {
	return &FileRestored{
		BasicEvent: event.NewBasicEvent(FileRestoredEventName, uuid.New(), snapshotID),
		filePath:   filePath,
	}
}

type SnapshotForgotten struct {
	event.BasicEvent
	createdAt time.Time
	source    string
}

func (s *SnapshotForgotten) SnapshotID() string {
	return s.AggregateRootID()
}

func (s *SnapshotForgotten) CreatedAt() time.Time {
	return s.createdAt
}

func (s *SnapshotForgotten) Source() string {
	return s.source
}

func (d *RetentionDecision) Forgotten() *SnapshotForgotten {
	return &SnapshotForgotten{
		BasicEvent: event.NewBasicEvent(SnapshotForgottenEventName, uuid.New(), d.snapshot.ID()),
		createdAt:  d.snapshot.CreatedAt(),
		source:     d.snapshot.Source(),
	}
}

type ObjectsPruned struct {
	event.BasicEvent
	removed int
}

func (o *ObjectsPruned) RepositoryID() string {
	return o.AggregateRootID()
}

func (o *ObjectsPruned) Removed() int {
	return o.removed
}

func (p *PrunePlan) Pruned(repositoryID string) *ObjectsPruned {
	return &ObjectsPruned{
		BasicEvent: event.NewBasicEvent(ObjectsPrunedEventName, uuid.New(), repositoryID),
		removed:    len(p.unreferenced),
	}
}

type MissingObjectsDetected struct {
	event.BasicEvent
	hashes []string
}

func (m *MissingObjectsDetected) RepositoryID() string {
	return m.AggregateRootID()
}

func (m *MissingObjectsDetected) Hashes() []string {
	return m.hashes
}

func (p *PrunePlan) MissingDetected(repositoryID string) *MissingObjectsDetected {
	return &MissingObjectsDetected{
		BasicEvent: event.NewBasicEvent(MissingObjectsDetectedEventName, uuid.New(), repositoryID),
		hashes:     p.missing,
	}
}

type GenerationsRemoved struct {
	event.BasicEvent
	generations []uint64
}

func (g *GenerationsRemoved) RepositoryID() string {
	return g.AggregateRootID()
}

func (g *GenerationsRemoved) Generations() []uint64 {
	return g.generations
}

func (p *PrunePlan) GenerationsRemoved(repositoryID string) *GenerationsRemoved {
	return &GenerationsRemoved{
		BasicEvent:  event.NewBasicEvent(GenerationsRemovedEventName, uuid.New(), repositoryID),
		generations: p.generations,
	}
}

type TemporariesRemoved struct {
	event.BasicEvent
	removed int
}

func (t *TemporariesRemoved) RepositoryID() string {
	return t.AggregateRootID()
}

func (t *TemporariesRemoved) Removed() int {
	return t.removed
}

func NewTemporariesRemoved(repositoryID string, removed int) *TemporariesRemoved {
	return &TemporariesRemoved{
		BasicEvent: event.NewBasicEvent(TemporariesRemovedEventName, uuid.New(), repositoryID),
		removed:    removed,
	}
}
