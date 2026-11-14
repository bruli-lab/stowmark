package stowmark

import (
	"context"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
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
	obsv, err := buildObservability(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	mdw, err := middlewares.BuildQueryMiddlewares(obsv)
	if err != nil {
		return nil, err
	}
	handler := mdw(app.NewCheckRepository(
		h.folderRepository, h.manifestRepository, h.objectRepository, h.lockRepository,
		h.asymmetricKeyPairRepository, h.symmetricKeyRepository, h.locker,
	))
	result, err := handler.Handle(ctx, app.CheckRepositoryQuery{
		RepositoryPath: h.repositoryPath,
		ReadData:       options.ReadData,
		Subset:         subset,
		PrivateKeyPath: options.PrivateKey,
	})
	if err != nil {
		return nil, err
	}
	report, ok := result.(*snapshot.CheckReport)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
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
