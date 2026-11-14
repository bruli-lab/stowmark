package stowmark

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
)

type RetentionPolicy struct {
	KeepLast    int
	KeepDaily   int
	KeepWeekly  int
	KeepMonthly int
	KeepYearly  int
	KeepWithin  time.Duration
}

type SnapshotRetention struct {
	ID        string
	CreatedAt time.Time
	Source    string
	Keep      bool
	Reasons   []string
}

type ForgetResult struct {
	Snapshots []SnapshotRetention
	DryRun    bool
}

func (h *Handler) Forget(ctx context.Context, policy RetentionPolicy, dryRun bool) (*ForgetResult, error) {
	retention, err := snapshot.NewRetentionPolicy(snapshot.RetentionRules{
		Last:    policy.KeepLast,
		Daily:   policy.KeepDaily,
		Weekly:  policy.KeepWeekly,
		Monthly: policy.KeepMonthly,
		Yearly:  policy.KeepYearly,
		Within:  policy.KeepWithin,
	})
	if err != nil {
		return nil, err
	}
	obsv, err := buildObservability(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	mdw, err := middlewares.BuildCommandMiddlewares(obsv, app.NewEventsTracing())
	if err != nil {
		return nil, err
	}
	events, err := mdw(app.NewForgetSnapshots(h.manifestRepository, h.locker)).Handle(ctx, app.ForgetSnapshotsCommand{
		RepositoryPath: h.repositoryPath,
		Policy:         retention,
		DryRun:         dryRun,
	})
	if err != nil {
		return nil, err
	}
	if len(events) != 1 {
		return nil, errors.New("unexpected number of events")
	}
	result, ok := events[0].(*app.ForgetSnapshotsEvent)
	if !ok {
		return nil, fmt.Errorf("unexpected event type: %T", events[0])
	}

	snapshots := make([]SnapshotRetention, len(result.Decisions))
	for i, decision := range result.Decisions {
		item := decision.Snapshot()
		reasons := make([]string, len(decision.Reasons()))
		for j, reason := range decision.Reasons() {
			reasons[j] = string(reason)
		}
		snapshots[i] = SnapshotRetention{
			ID:        item.ID(),
			CreatedAt: item.CreatedAt(),
			Source:    item.Source(),
			Keep:      decision.Keep(),
			Reasons:   reasons,
		}
	}

	return &ForgetResult{Snapshots: snapshots, DryRun: result.DryRun}, nil
}
