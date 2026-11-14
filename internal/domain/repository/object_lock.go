package repository

import (
	"errors"
	"fmt"
	"time"
)

type ObjectLockMode string

const (
	GovernanceObjectLock ObjectLockMode = "governance"
	ComplianceObjectLock ObjectLockMode = "compliance"
)

var (
	ErrInvalidObjectLock     = errors.New("invalid object lock")
	ErrObjectLockUnsupported = errors.New("the repository backend does not support object lock")
	ErrObjectLockDisabled    = errors.New("the repository does not use object lock")
	ErrRetained              = errors.New("still under retention")
)

func ParseObjectLockMode(value string) (ObjectLockMode, error) {
	switch mode := ObjectLockMode(value); mode {
	case GovernanceObjectLock, ComplianceObjectLock:
		return mode, nil
	default:
		return "", fmt.Errorf("%w: unknown mode %q, use governance or compliance", ErrInvalidObjectLock, value)
	}
}

type ObjectLock struct {
	mode   ObjectLockMode
	period time.Duration
}

func NewObjectLock(mode ObjectLockMode, period time.Duration) (*ObjectLock, error) {
	if _, err := ParseObjectLockMode(string(mode)); err != nil {
		return nil, err
	}
	if period < time.Second {
		return nil, fmt.Errorf("%w: the retention period must be at least one second, got %s", ErrInvalidObjectLock, period)
	}
	return &ObjectLock{mode: mode, period: period.Truncate(time.Second)}, nil
}

func (o *ObjectLock) Mode() ObjectLockMode {
	return o.mode
}

func (o *ObjectLock) Period() time.Duration {
	return o.period
}

func (o *ObjectLock) RetentionFrom(now time.Time) Retention {
	return Retention{mode: o.mode, until: now.Add(o.period).UTC().Truncate(time.Second)}
}

func (o *ObjectLock) NeedsExtension(until, now time.Time) bool {
	return until.Before(now.Add(o.period / 2))
}

type Retention struct {
	mode  ObjectLockMode
	until time.Time
}

func NewRetention(mode ObjectLockMode, until time.Time) Retention {
	return Retention{mode: mode, until: until.UTC()}
}

func (r Retention) Mode() ObjectLockMode {
	return r.mode
}

func (r Retention) Until() time.Time {
	return r.until
}

type RetainedError struct {
	until time.Time
}

func NewRetainedError(until time.Time) *RetainedError {
	return &RetainedError{until: until.UTC()}
}

func (r *RetainedError) Error() string {
	if r.until.IsZero() {
		return ErrRetained.Error()
	}
	return fmt.Sprintf("%s until %s", ErrRetained, r.until.Format(time.RFC3339))
}

func (r *RetainedError) Unwrap() error {
	return ErrRetained
}

func (r *RetainedError) Until() time.Time {
	return r.until
}

func RetainedUntil(err error) (time.Time, bool) {
	var retained *RetainedError
	if errors.As(err, &retained) {
		return retained.until, true
	}
	return time.Time{}, errors.Is(err, ErrRetained)
}

type RetainedItemKind string

const (
	RetainedObject   RetainedItemKind = "object"
	RetainedManifest RetainedItemKind = "manifest"
	RetainedConfig   RetainedItemKind = "config"
)

type RetainedItem struct {
	kind       RetainedItemKind
	id         string
	encrypted  bool
	generation uint64
}

func NewRetainedObject(hash string, encrypted bool, generation uint64) RetainedItem {
	return RetainedItem{kind: RetainedObject, id: hash, encrypted: encrypted, generation: generation}
}

func NewRetainedManifest(snapshotID string) RetainedItem {
	return RetainedItem{kind: RetainedManifest, id: snapshotID}
}

func NewRetainedConfig() RetainedItem {
	return RetainedItem{kind: RetainedConfig}
}

func (r RetainedItem) Kind() RetainedItemKind {
	return r.kind
}

func (r RetainedItem) ID() string {
	return r.id
}

func (r RetainedItem) Encrypted() bool {
	return r.encrypted
}

func (r RetainedItem) Generation() uint64 {
	return r.generation
}

func (c Config) RetainedObject(hash string) RetainedItem {
	if c.encryption == nil {
		return NewRetainedObject(hash, false, 0)
	}
	return NewRetainedObject(hash, true, c.encryption.Generation())
}
