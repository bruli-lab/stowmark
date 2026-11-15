package snapshot

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

type RetentionReason string

const (
	KeepLastReason    RetentionReason = "last"
	KeepDailyReason   RetentionReason = "daily"
	KeepWeeklyReason  RetentionReason = "weekly"
	KeepMonthlyReason RetentionReason = "monthly"
	KeepYearlyReason  RetentionReason = "yearly"
	KeepWithinReason  RetentionReason = "within"
)

var (
	ErrEmptyRetentionPolicy    = errors.New("retention policy must keep at least one snapshot")
	ErrNegativeRetentionPolicy = errors.New("retention policy values must not be negative")
)

type RetentionRules struct {
	Last    int
	Daily   int
	Weekly  int
	Monthly int
	Yearly  int
	Within  time.Duration
}

type RetentionPolicy struct {
	rules RetentionRules
}

func NewRetentionPolicy(rules RetentionRules) (*RetentionPolicy, error) {
	counts := []int{rules.Last, rules.Daily, rules.Weekly, rules.Monthly, rules.Yearly}
	if slices.ContainsFunc(counts, func(count int) bool { return count < 0 }) || rules.Within < 0 {
		return nil, fmt.Errorf("%w: %+v", ErrNegativeRetentionPolicy, rules)
	}
	if !slices.ContainsFunc(counts, func(count int) bool { return count > 0 }) && rules.Within == 0 {
		return nil, ErrEmptyRetentionPolicy
	}
	return &RetentionPolicy{rules: rules}, nil
}

func (p *RetentionPolicy) Rules() RetentionRules {
	return p.rules
}

type RetentionDecision struct {
	snapshot ManifestResume
	reasons  []RetentionReason
}

func (d *RetentionDecision) Snapshot() ManifestResume {
	return d.snapshot
}

func (d *RetentionDecision) Reasons() []RetentionReason {
	return d.reasons
}

func (d *RetentionDecision) Keep() bool {
	return len(d.reasons) > 0
}

func (p *RetentionPolicy) Apply(snapshots []ManifestResume, now time.Time) []RetentionDecision {
	ordered := slices.Clone(snapshots)
	slices.SortStableFunc(ordered, func(a, b ManifestResume) int {
		if byDate := b.CreatedAt().Compare(a.CreatedAt()); byDate != 0 {
			return byDate
		}
		return strings.Compare(b.ID(), a.ID())
	})

	decisions := make([]RetentionDecision, len(ordered))
	groups := map[string][]int{}
	for i := range ordered {
		decisions[i].snapshot = ordered[i]
		groups[ordered[i].Source()] = append(groups[ordered[i].Source()], i)
	}
	for _, indexes := range groups {
		p.applyToGroup(decisions, indexes, now)
	}

	return decisions
}

func (p *RetentionPolicy) applyToGroup(decisions []RetentionDecision, newestFirst []int, now time.Time) {
	buckets := []*retentionBucket{
		{reason: KeepDailyReason, limit: p.rules.Daily, period: dayPeriod},
		{reason: KeepWeeklyReason, limit: p.rules.Weekly, period: weekPeriod},
		{reason: KeepMonthlyReason, limit: p.rules.Monthly, period: monthPeriod},
		{reason: KeepYearlyReason, limit: p.rules.Yearly, period: yearPeriod},
	}
	withinSince := now.Add(-p.rules.Within)

	for position, index := range newestFirst {
		decision := &decisions[index]
		createdAt := decision.snapshot.CreatedAt().UTC()

		if position < p.rules.Last {
			decision.reasons = append(decision.reasons, KeepLastReason)
		}
		for _, bucket := range buckets {
			if bucket.keeps(createdAt) {
				decision.reasons = append(decision.reasons, bucket.reason)
			}
		}
		if p.rules.Within > 0 && !createdAt.Before(withinSince) {
			decision.reasons = append(decision.reasons, KeepWithinReason)
		}
	}
}

type retentionBucket struct {
	reason     RetentionReason
	limit      int
	period     func(time.Time) string
	lastPeriod string
	kept       int
}

func (b *retentionBucket) keeps(createdAt time.Time) bool {
	if b.kept >= b.limit {
		return false
	}
	period := b.period(createdAt)
	if period == b.lastPeriod {
		return false
	}
	b.lastPeriod = period
	b.kept++
	return true
}

func dayPeriod(t time.Time) string {
	return t.Format(time.DateOnly)
}

func weekPeriod(t time.Time) string {
	year, week := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", year, week)
}

func monthPeriod(t time.Time) string {
	return t.Format("2006-01")
}

func yearPeriod(t time.Time) string {
	return t.Format("2006")
}
