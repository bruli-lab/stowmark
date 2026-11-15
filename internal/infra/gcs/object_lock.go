package gcs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
)

const (
	objectRetentionEnabled = "Enabled"
	unlockedRetention      = "Unlocked"
	lockedRetention        = "Locked"
)

var (
	ErrObjectRetentionNotEnabled = errors.New("the bucket does not have object retention enabled; create it with object retention")
	ErrVersioningNotEnabled      = errors.New("the bucket does not have object versioning enabled; the repository config can only be rewritten as a new version")
	ErrBucketRetentionPolicy     = errors.New("the bucket has a retention policy, which would also retain the repository locks; remove it, Stowmark sets the retention of each object")
)

type objectLockState struct {
	once    sync.Once
	enabled bool
}

func (s BlobStore) VerifyObjectLock(ctx context.Context) error {
	attrs, err := s.client.Bucket(s.bucket).Attrs(ctx)
	if err != nil {
		return fmt.Errorf("read the attributes of bucket %q: %w", s.bucket, err)
	}
	if attrs.ObjectRetentionMode != objectRetentionEnabled {
		return ErrObjectRetentionNotEnabled
	}
	if !attrs.VersioningEnabled {
		return ErrVersioningNotEnabled
	}
	if attrs.RetentionPolicy != nil && attrs.RetentionPolicy.RetentionPeriod > 0 {
		return ErrBucketRetentionPolicy
	}
	return nil
}

func (s BlobStore) PutRetained(ctx context.Context, key string, retention repository.Retention, write func(io.Writer) error) error {
	return s.put(ctx, key, write, func(writer *storage.Writer) {
		writer.Retention = objectRetention(retention)
	})
}

func (s BlobStore) RetainedUntil(ctx context.Context, key string) (time.Time, error) {
	objectName, err := s.objectName(ctx, key)
	if err != nil {
		return time.Time{}, err
	}
	attrs, err := s.client.Bucket(s.bucket).Object(objectName).Attrs(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return time.Time{}, fmt.Errorf("%w: %q", blob.ErrNotFound, key)
		}
		return time.Time{}, fmt.Errorf("read the retention of %q in bucket %q: %w", objectName, s.bucket, err)
	}
	if attrs.Retention == nil {
		return time.Time{}, nil
	}
	return attrs.Retention.RetainUntil, nil
}

func (s BlobStore) Retain(ctx context.Context, key string, retention repository.Retention) error {
	objectName, err := s.objectName(ctx, key)
	if err != nil {
		return err
	}
	_, err = s.client.Bucket(s.bucket).Object(objectName).Update(ctx, storage.ObjectAttrsToUpdate{Retention: objectRetention(retention)})
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return fmt.Errorf("%w: %q", blob.ErrNotFound, key)
		}
		return fmt.Errorf("extend the retention of %q in bucket %q: %w", objectName, s.bucket, err)
	}
	return nil
}

func (s BlobStore) usesObjectLock(ctx context.Context) bool {
	s.lock.once.Do(func() {
		attrs, err := s.client.Bucket(s.bucket).Attrs(ctx)
		s.lock.enabled = err == nil && attrs.ObjectRetentionMode == objectRetentionEnabled
	})
	return s.lock.enabled
}

func (s BlobStore) deleteRetainable(ctx context.Context, objectName string) error {
	if !s.usesObjectLock(ctx) {
		return s.deleteCurrent(ctx, objectName)
	}
	return s.deleteGenerations(ctx, objectName)
}

func (s BlobStore) deleteCurrent(ctx context.Context, objectName string) error {
	if err := s.client.Bucket(s.bucket).Object(objectName).Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		return fmt.Errorf("delete object %q from bucket %q: %w", objectName, s.bucket, err)
	}
	return nil
}

func (s BlobStore) deleteGenerations(ctx context.Context, objectName string) error {
	var latest time.Time
	retained := false
	generations := s.client.Bucket(s.bucket).Objects(ctx, &storage.Query{Prefix: objectName, Versions: true})
	for {
		attrs, err := generations.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return fmt.Errorf("list the generations of %q in bucket %q: %w", objectName, s.bucket, err)
		}
		if attrs.Name != objectName {
			continue
		}
		until, err := s.deleteGeneration(ctx, objectName, attrs.Generation)
		if err != nil {
			return err
		}
		if !until.IsZero() {
			retained = true
			latest = maxTime(latest, until)
		}
	}
	if retained {
		return repository.NewRetainedError(latest)
	}
	return nil
}

func (s BlobStore) deleteGeneration(ctx context.Context, objectName string, generation int64) (time.Time, error) {
	object := s.client.Bucket(s.bucket).Object(objectName).Generation(generation)
	err := object.Delete(ctx)
	if err == nil || errors.Is(err, storage.ErrObjectNotExist) {
		return time.Time{}, nil
	}
	if !isForbidden(err) {
		return time.Time{}, fmt.Errorf("delete generation %d of %q from bucket %q: %w", generation, objectName, s.bucket, err)
	}
	attrs, attrsErr := object.Attrs(ctx)
	if attrsErr != nil || attrs.Retention == nil || !attrs.Retention.RetainUntil.After(time.Now()) {
		return time.Time{}, fmt.Errorf("delete generation %d of %q from bucket %q: %w", generation, objectName, s.bucket, err)
	}
	return attrs.Retention.RetainUntil, nil
}

func objectRetention(retention repository.Retention) *storage.ObjectRetention {
	mode := unlockedRetention
	if retention.Mode() == repository.ComplianceObjectLock {
		mode = lockedRetention
	}
	return &storage.ObjectRetention{Mode: mode, RetainUntil: retention.Until()}
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func isForbidden(err error) bool {
	if apiErr, ok := errors.AsType[*googleapi.Error](err); ok {
		return apiErr.Code == http.StatusForbidden
	}
	return false
}
