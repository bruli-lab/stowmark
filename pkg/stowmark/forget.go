package stowmark

import (
	"context"
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

func (r RetentionPolicy) build() (*snapshot.RetentionPolicy, error) {
	return snapshot.NewRetentionPolicy(snapshot.RetentionRules{
		Last:    r.KeepLast,
		Daily:   r.KeepDaily,
		Weekly:  r.KeepWeekly,
		Monthly: r.KeepMonthly,
		Yearly:  r.KeepYearly,
		Within:  r.KeepWithin,
	})
}

type SnapshotRetention struct {
	ID        string
	CreatedAt time.Time
	Source    string
	Keep      bool
	Reasons   []string
}

type ForgottenSnapshot struct {
	ID        string
	CreatedAt time.Time
	Source    string
}

func (h *Handler) PlanForget(ctx context.Context, policy RetentionPolicy) ([]SnapshotRetention, error) {
	retention, err := policy.build()
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
	mdw, err := middlewares.BuildQueryMiddlewares(obsv)
	if err != nil {
		return nil, err
	}
	result, err := mdw(app.NewForgetPlan(h.manifestRepository, h.locker)).Handle(ctx, app.ForgetPlanQuery{Policy: retention})
	if err != nil {
		return nil, err
	}
	decisions, ok := result.([]snapshot.RetentionDecision)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	snapshots := make([]SnapshotRetention, len(decisions))
	for i := range decisions {
		item := decisions[i].Snapshot()
		reasons := make([]string, len(decisions[i].Reasons()))
		for j, reason := range decisions[i].Reasons() {
			reasons[j] = string(reason)
		}
		snapshots[i] = SnapshotRetention{
			ID:        item.ID(),
			CreatedAt: item.CreatedAt(),
			Source:    item.Source(),
			Keep:      decisions[i].Keep(),
			Reasons:   reasons,
		}
	}

	return snapshots, nil
}

func (h *Handler) Forget(ctx context.Context, policy RetentionPolicy) ([]ForgottenSnapshot, error) {
	retention, err := policy.build()
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
	})
	if err != nil {
		return nil, err
	}

	forgotten := app.EventsOf[*snapshot.SnapshotForgotten](events)
	result := make([]ForgottenSnapshot, len(forgotten))
	for i, ev := range forgotten {
		result[i] = ForgottenSnapshot{ID: ev.SnapshotID(), CreatedAt: ev.CreatedAt(), Source: ev.Source()}
	}

	return result, nil
}
