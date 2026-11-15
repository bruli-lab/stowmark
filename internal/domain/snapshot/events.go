package snapshot

import (
	"slices"
	"time"

	"github.com/bruli-lab/go-core/event"
	"github.com/google/uuid"
)

const (
	SnapshotCreatedEventName          = "snapshot.created"
	SnapshotRestoredEventName         = "snapshot.restored"
	FileRestoredEventName             = "snapshot.file_restored"
	SnapshotForgottenEventName        = "snapshot.forgotten"
	ObjectsPrunedEventName            = "objects.pruned"
	MissingObjectsDetectedEventName   = "objects.missing_detected"
	GenerationsRemovedEventName       = "objects.generations_removed"
	TemporariesRemovedEventName       = "objects.temporaries_removed"
	SnapshotForgetPostponedEventName  = "snapshot.forget_postponed"
	ObjectsDeletionPostponedEventName = "objects.deletion_postponed"
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

type SnapshotForgetPostponed struct {
	event.BasicEvent
	createdAt time.Time
	source    string
	until     time.Time
}

func (s *SnapshotForgetPostponed) SnapshotID() string {
	return s.AggregateRootID()
}

func (s *SnapshotForgetPostponed) CreatedAt() time.Time {
	return s.createdAt
}

func (s *SnapshotForgetPostponed) Source() string {
	return s.source
}

func (s *SnapshotForgetPostponed) Until() time.Time {
	return s.until
}

func (d *RetentionDecision) ForgetPostponed(until time.Time) *SnapshotForgetPostponed {
	return &SnapshotForgetPostponed{
		BasicEvent: event.NewBasicEvent(SnapshotForgetPostponedEventName, uuid.New(), d.snapshot.ID()),
		createdAt:  d.snapshot.CreatedAt(),
		source:     d.snapshot.Source(),
		until:      until.UTC(),
	}
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
	removed    int
	freedBytes int64
}

func (o *ObjectsPruned) RepositoryID() string {
	return o.AggregateRootID()
}

func (o *ObjectsPruned) Removed() int {
	return o.removed
}

func (o *ObjectsPruned) FreedBytes() int64 {
	return o.freedBytes
}

func (p *PrunePlan) Pruned(repositoryID string) *ObjectsPruned {
	return p.PrunedExcept(repositoryID, nil)
}

func (p *PrunePlan) PrunedExcept(repositoryID string, postponed []string) *ObjectsPruned {
	return &ObjectsPruned{
		BasicEvent: event.NewBasicEvent(ObjectsPrunedEventName, uuid.New(), repositoryID),
		removed:    len(p.unreferenced) - len(postponed),
		freedBytes: p.unreferencedBytes - p.sizeOf(postponed),
	}
}

func (p *PrunePlan) sizeOf(hashes []string) int64 {
	var size int64
	for _, hash := range hashes {
		size += p.unreferencedSizes[hash]
	}
	return size
}

type ObjectsDeletionPostponed struct {
	event.BasicEvent
	objects     int
	bytes       int64
	generations []uint64
	until       time.Time
}

func (o *ObjectsDeletionPostponed) RepositoryID() string {
	return o.AggregateRootID()
}

func (o *ObjectsDeletionPostponed) Objects() int {
	return o.objects
}

func (o *ObjectsDeletionPostponed) Bytes() int64 {
	return o.bytes
}

func (o *ObjectsDeletionPostponed) Generations() []uint64 {
	return o.generations
}

func (o *ObjectsDeletionPostponed) Until() time.Time {
	return o.until
}

func (p *PrunePlan) DeletionPostponed(repositoryID string, objects []string, generations []uint64, until time.Time) *ObjectsDeletionPostponed {
	return &ObjectsDeletionPostponed{
		BasicEvent:  event.NewBasicEvent(ObjectsDeletionPostponedEventName, uuid.New(), repositoryID),
		objects:     len(objects),
		bytes:       p.sizeOf(objects),
		generations: generations,
		until:       until.UTC(),
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
	return p.GenerationsRemovedExcept(repositoryID, nil)
}

func (p *PrunePlan) GenerationsRemovedExcept(repositoryID string, postponed []uint64) *GenerationsRemoved {
	removed := make([]uint64, 0, len(p.generations))
	for _, generation := range p.generations {
		if !slices.Contains(postponed, generation) {
			removed = append(removed, generation)
		}
	}
	return &GenerationsRemoved{
		BasicEvent:  event.NewBasicEvent(GenerationsRemovedEventName, uuid.New(), repositoryID),
		generations: removed,
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
