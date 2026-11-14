package stowmark

import (
	"context"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
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
	decisions, err := app.QueryResult[[]snapshot.RetentionDecision](h.repository.ForgetPlan().Handle(ctx, app.ForgetPlanQuery{Policy: retention}))
	if err != nil {
		return nil, err
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
	events, err := h.repository.ForgetSnapshots().Handle(ctx, app.ForgetSnapshotsCommand{
		RepositoryPath: h.repository.Path(),
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
