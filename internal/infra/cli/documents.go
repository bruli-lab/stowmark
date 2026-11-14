package cli

import (
	"fmt"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/model"
)

type initDocument struct {
	RepositoryID string   `json:"repository_id"`
	Path         string   `json:"path"`
	Warnings     []string `json:"warnings"`
}

type skippedDocument struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type createDocument struct {
	SnapshotID string            `json:"snapshot_id"`
	Files      int               `json:"files"`
	Size       int64             `json:"size"`
	Skipped    []skippedDocument `json:"skipped"`
}

type snapshotDocument struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Files     int       `json:"files"`
	Size      int64     `json:"size"`
	Source    string    `json:"source"`
}

type entryDocument struct {
	Path       string     `json:"path"`
	Kind       string     `json:"kind"`
	Hash       string     `json:"hash,omitempty"`
	Size       int64      `json:"size"`
	Target     string     `json:"target,omitempty"`
	Mode       *string    `json:"mode,omitempty"`
	ModifiedAt *time.Time `json:"modified_at,omitempty"`
}

type manifestDocument struct {
	ID          string          `json:"id"`
	CreatedAt   time.Time       `json:"created_at"`
	Source      string          `json:"source"`
	Compression string          `json:"compression"`
	Entries     []entryDocument `json:"entries"`
}

type failedDocument struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type resultDocument struct {
	SnapshotID string           `json:"snapshot_id"`
	Success    bool             `json:"success"`
	Files      int              `json:"files"`
	Valid      int              `json:"valid"`
	Invalid    int              `json:"invalid"`
	Failed     []failedDocument `json:"failed"`
}

type restoredFileDocument struct {
	SnapshotID string `json:"snapshot_id"`
	File       string `json:"file"`
}

type manifestProblemDocument struct {
	SnapshotID string `json:"snapshot_id"`
	Reason     string `json:"reason"`
}

type missingObjectDocument struct {
	Hash      string   `json:"hash"`
	Snapshots []string `json:"snapshots"`
}

type objectProblemDocument struct {
	Hash   string `json:"hash"`
	Reason string `json:"reason"`
}

type checkDocument struct {
	Success             bool                      `json:"success"`
	Snapshots           int                       `json:"snapshots"`
	CorruptManifests    []manifestProblemDocument `json:"corrupt_manifests"`
	StoredObjects       int                       `json:"stored_objects"`
	ReferencedObjects   int                       `json:"referenced_objects"`
	MissingObjects      []missingObjectDocument   `json:"missing_objects"`
	UnreferencedObjects int                       `json:"unreferenced_objects"`
	StaleLocks          int                       `json:"stale_locks"`
	OldKeyGenerations   []uint64                  `json:"old_key_generations"`
	ReadData            bool                      `json:"read_data"`
	ReadObjects         int                       `json:"read_objects"`
	CorruptObjects      []objectProblemDocument   `json:"corrupt_objects"`
}

type retentionDocument struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source"`
	Keep      bool      `json:"keep"`
	Reasons   []string  `json:"reasons"`
}

type forgottenDocument struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source"`
}

type forgetDocument struct {
	Forgotten []forgottenDocument `json:"forgotten"`
	Prune     *pruneDocument      `json:"prune,omitempty"`
}

type prunePlanDocument struct {
	StoredObjects       int      `json:"stored_objects"`
	ReferencedObjects   int      `json:"referenced_objects"`
	UnreferencedObjects int      `json:"unreferenced_objects"`
	UnreferencedBytes   int64    `json:"unreferenced_bytes"`
	MissingObjects      int      `json:"missing_objects"`
	OldKeyGenerations   []uint64 `json:"old_key_generations"`
}

type pruneDocument struct {
	RemovedObjects        int      `json:"removed_objects"`
	FreedBytes            int64    `json:"freed_bytes"`
	RemovedKeyGenerations []uint64 `json:"removed_key_generations"`
	RemovedTemporaryFiles int      `json:"removed_temporary_files"`
	MissingObjects        int      `json:"missing_objects"`
}

type unlockDocument struct {
	Removed int `json:"removed"`
	Active  int `json:"active"`
}

type rekeyDocument struct {
	RepositoryID   string `json:"repository_id"`
	Generation     uint64 `json:"generation"`
	RekeyedObjects int    `json:"rekeyed_objects"`
}

type rewrapDocument struct {
	RepositoryID         string `json:"repository_id"`
	PublicKeyFingerprint string `json:"public_key_fingerprint"`
}

type keyPairDocument struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}

func newInitDocument(path string, initialized *repository.RepositoryInitialized) initDocument {
	return initDocument{
		RepositoryID: initialized.RepositoryID(),
		Path:         path,
		Warnings:     nonNil(initialized.Warnings()),
	}
}

func newCreateDocument(created *snapshot.SnapshotCreated) createDocument {
	skipped := make([]skippedDocument, len(created.SkippedFiles()))
	for i, file := range created.SkippedFiles() {
		skipped[i] = skippedDocument{Path: file.Path(), Kind: file.Kind()}
	}
	return createDocument{
		SnapshotID: created.SnapshotID(),
		Files:      created.FileCount(),
		Size:       created.TotalSize(),
		Skipped:    skipped,
	}
}

func newSnapshotDocuments(snapshots []snapshot.ManifestResume) []snapshotDocument {
	documents := make([]snapshotDocument, len(snapshots))
	for i, item := range snapshots {
		documents[i] = snapshotDocument{
			ID:        item.ID(),
			CreatedAt: item.CreatedAt().UTC(),
			Files:     item.Files(),
			Size:      item.Size(),
			Source:    item.Source(),
		}
	}
	return documents
}

func newManifestDocument(manifest *snapshot.Manifest) manifestDocument {
	entries := make([]entryDocument, len(manifest.Files()))
	for i, file := range manifest.Files() {
		entries[i] = newEntryDocument(file)
	}
	compression := repository.NoneCompressionType.String()
	if manifest.Compression() != nil {
		compression = manifest.Compression().CompType().String()
	}
	return manifestDocument{
		ID:          manifest.ID(),
		CreatedAt:   manifest.CreatedAt().UTC(),
		Source:      manifest.Source(),
		Compression: compression,
		Entries:     entries,
	}
}

func newEntryDocument(file snapshot.File) entryDocument {
	entry := entryDocument{
		Path:   file.Path(),
		Kind:   string(file.Kind()),
		Hash:   file.Hash(),
		Size:   file.Size(),
		Target: file.Target(),
	}
	if metadata, ok := file.Metadata(); ok {
		mode := fmt.Sprintf("%04o", model.UnixMode(metadata.Mode()))
		modifiedAt := metadata.ModTime().UTC()
		entry.Mode = &mode
		entry.ModifiedAt = &modifiedAt
	}
	return entry
}

func newResultDocument(snapshotID string, totalFiles int, failed []snapshot.FailedResult, success bool) resultDocument {
	documents := make([]failedDocument, len(failed))
	for i, result := range failed {
		documents[i] = failedDocument{Path: result.Path(), Reason: result.Reason()}
	}
	return resultDocument{
		SnapshotID: snapshotID,
		Success:    success,
		Files:      totalFiles,
		Valid:      totalFiles - len(failed),
		Invalid:    len(failed),
		Failed:     documents,
	}
}

func newCheckDocument(report *snapshot.CheckReport) checkDocument {
	manifests := make([]manifestProblemDocument, len(report.CorruptManifests()))
	for i, problem := range report.CorruptManifests() {
		manifests[i] = manifestProblemDocument{SnapshotID: problem.SnapshotID(), Reason: problem.Reason()}
	}
	missing := make([]missingObjectDocument, len(report.MissingObjects()))
	for i, object := range report.MissingObjects() {
		missing[i] = missingObjectDocument{Hash: object.Hash(), Snapshots: nonNil(object.Snapshots())}
	}
	corrupt := make([]objectProblemDocument, len(report.CorruptObjects()))
	for i, problem := range report.CorruptObjects() {
		corrupt[i] = objectProblemDocument{Hash: problem.Hash(), Reason: problem.Reason()}
	}
	return checkDocument{
		Success:             !report.HasErrors(),
		Snapshots:           report.Snapshots(),
		CorruptManifests:    manifests,
		StoredObjects:       report.StoredObjects(),
		ReferencedObjects:   report.ReferencedObjects(),
		MissingObjects:      missing,
		UnreferencedObjects: report.UnreferencedObjects(),
		StaleLocks:          report.StaleLocks(),
		OldKeyGenerations:   nonNil(report.OrphanGenerations()),
		ReadData:            report.ReadData(),
		ReadObjects:         report.ReadObjects(),
		CorruptObjects:      corrupt,
	}
}

func newRetentionDocuments(decisions []snapshot.RetentionDecision) []retentionDocument {
	documents := make([]retentionDocument, len(decisions))
	for i := range decisions {
		item := decisions[i].Snapshot()
		reasons := make([]string, len(decisions[i].Reasons()))
		for j, reason := range decisions[i].Reasons() {
			reasons[j] = string(reason)
		}
		documents[i] = retentionDocument{
			ID:        item.ID(),
			CreatedAt: item.CreatedAt().UTC(),
			Source:    item.Source(),
			Keep:      decisions[i].Keep(),
			Reasons:   reasons,
		}
	}
	return documents
}

func newForgottenDocuments(forgotten []*snapshot.SnapshotForgotten) []forgottenDocument {
	documents := make([]forgottenDocument, len(forgotten))
	for i, ev := range forgotten {
		documents[i] = forgottenDocument{ID: ev.SnapshotID(), CreatedAt: ev.CreatedAt().UTC(), Source: ev.Source()}
	}
	return documents
}

func newPrunePlanDocument(plan *snapshot.PrunePlan) prunePlanDocument {
	return prunePlanDocument{
		StoredObjects:       plan.StoredObjects(),
		ReferencedObjects:   plan.ReferencedObjects(),
		UnreferencedObjects: len(plan.Unreferenced()),
		UnreferencedBytes:   plan.UnreferencedBytes(),
		MissingObjects:      len(plan.Missing()),
		OldKeyGenerations:   nonNil(plan.OrphanGenerations()),
	}
}

func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}
