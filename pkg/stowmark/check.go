package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

type CheckOptions struct {
	ReadData       bool
	ReadDataSubset string
	PrivateKey     *string
}

type CheckProblem struct {
	ID     string
	Reason string
}

type MissingObject struct {
	Hash      string
	Snapshots []string
}

type CheckResult struct {
	Snapshots           int
	CorruptManifests    []CheckProblem
	StoredObjects       int
	ReferencedObjects   int
	UnreferencedObjects int
	MissingObjects      []MissingObject
	StaleLocks          int
	OldKeyGenerations   []uint64
	ReadData            bool
	ReadObjects         int
	CorruptObjects      []CheckProblem
	HasErrors           bool
}

func (h *Handler) Check(ctx context.Context, options CheckOptions) (*CheckResult, error) {
	var subset *snapshot.DataSubset
	if options.ReadDataSubset != "" {
		parsed, err := snapshot.ParseDataSubset(options.ReadDataSubset)
		if err != nil {
			return nil, err
		}
		subset = parsed
	}
	report, err := app.QueryResult[*snapshot.CheckReport](h.repository.CheckRepository().Handle(ctx, app.CheckRepositoryQuery{
		RepositoryPath: h.repository.Path(),
		ReadData:       options.ReadData,
		Subset:         subset,
		PrivateKeyPath: options.PrivateKey,
	}))
	if err != nil {
		return nil, err
	}

	corruptManifests := make([]CheckProblem, len(report.CorruptManifests()))
	for i, problem := range report.CorruptManifests() {
		corruptManifests[i] = CheckProblem{ID: problem.SnapshotID(), Reason: problem.Reason()}
	}
	missing := make([]MissingObject, len(report.MissingObjects()))
	for i, object := range report.MissingObjects() {
		missing[i] = MissingObject{Hash: object.Hash(), Snapshots: object.Snapshots()}
	}
	corruptObjects := make([]CheckProblem, len(report.CorruptObjects()))
	for i, problem := range report.CorruptObjects() {
		corruptObjects[i] = CheckProblem{ID: problem.Hash(), Reason: problem.Reason()}
	}

	return &CheckResult{
		Snapshots:           report.Snapshots(),
		CorruptManifests:    corruptManifests,
		StoredObjects:       report.StoredObjects(),
		ReferencedObjects:   report.ReferencedObjects(),
		UnreferencedObjects: report.UnreferencedObjects(),
		MissingObjects:      missing,
		StaleLocks:          report.StaleLocks(),
		OldKeyGenerations:   report.OrphanGenerations(),
		ReadData:            report.ReadData(),
		ReadObjects:         report.ReadObjects(),
		CorruptObjects:      corruptObjects,
		HasErrors:           report.HasErrors(),
	}, nil
}
