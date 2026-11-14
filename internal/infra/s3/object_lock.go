package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/blob"
)

var (
	ErrObjectLockNotEnabled   = errors.New("the bucket does not have object lock enabled; create it with object lock")
	ErrBucketDefaultRetention = errors.New("the bucket has a default retention, which would also retain the repository locks; remove it, Stowmark sets the retention of each object")
)

type objectLockState struct {
	once    sync.Once
	enabled bool
}

func (s BlobStore) VerifyObjectLock(ctx context.Context) error {
	configuration, err := s.objectLockConfiguration(ctx)
	if err != nil {
		return err
	}
	if configuration == nil || configuration.ObjectLockEnabled != types.ObjectLockEnabledEnabled {
		return ErrObjectLockNotEnabled
	}
	if configuration.Rule != nil && configuration.Rule.DefaultRetention != nil {
		return ErrBucketDefaultRetention
	}
	return nil
}

func (s BlobStore) objectLockConfiguration(ctx context.Context) (*types.ObjectLockConfiguration, error) {
	output, err := s.client.GetObjectLockConfiguration(ctx, &s3.GetObjectLockConfigurationInput{Bucket: aws.String(s.bucket)})
	if err != nil {
		if isObjectLockNotConfigured(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read the object lock configuration of bucket %q: %w", s.bucket, err)
	}
	return output.ObjectLockConfiguration, nil
}

func (s BlobStore) PutRetained(ctx context.Context, key string, retention repository.Retention, write func(io.Writer) error) error {
	return s.put(ctx, key, write, func(input *s3.PutObjectInput) {
		input.ObjectLockMode = types.ObjectLockMode(objectLockMode(retention.Mode()))
		input.ObjectLockRetainUntilDate = aws.Time(retention.Until())
		input.ChecksumAlgorithm = types.ChecksumAlgorithmCrc32
	})
}

func (s BlobStore) RetainedUntil(ctx context.Context, key string) (time.Time, error) {
	objectKey, err := s.objectKey(ctx, key)
	if err != nil {
		return time.Time{}, err
	}
	output, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey)})
	if err != nil {
		if isNotFoundError(err) {
			return time.Time{}, fmt.Errorf("%w: %q", blob.ErrNotFound, key)
		}
		return time.Time{}, fmt.Errorf("read the retention of %q in bucket %q: %w", objectKey, s.bucket, err)
	}
	return aws.ToTime(output.ObjectLockRetainUntilDate), nil
}

func (s BlobStore) Retain(ctx context.Context, key string, retention repository.Retention) error {
	objectKey, err := s.objectKey(ctx, key)
	if err != nil {
		return err
	}
	_, err = s.client.PutObjectRetention(ctx, &s3.PutObjectRetentionInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
		Retention: &types.ObjectLockRetention{
			Mode:            types.ObjectLockRetentionMode(objectLockMode(retention.Mode())),
			RetainUntilDate: aws.Time(retention.Until()),
		},
	})
	if err != nil {
		if isNotFoundError(err) {
			return fmt.Errorf("%w: %q", blob.ErrNotFound, key)
		}
		return fmt.Errorf("extend the retention of %q in bucket %q: %w", objectKey, s.bucket, err)
	}
	return nil
}

func (s BlobStore) usesObjectLock(ctx context.Context) bool {
	s.lock.once.Do(func() {
		configuration, err := s.objectLockConfiguration(ctx)
		s.lock.enabled = err == nil && configuration != nil && configuration.ObjectLockEnabled == types.ObjectLockEnabledEnabled
	})
	return s.lock.enabled
}

func (s BlobStore) deleteRetainable(ctx context.Context, objectKey string) error {
	if !s.usesObjectLock(ctx) {
		return s.deleteCurrent(ctx, objectKey)
	}
	return s.deleteVersions(ctx, objectKey)
}

func (s BlobStore) deleteCurrent(ctx context.Context, objectKey string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil && !isNotFoundError(err) {
		return fmt.Errorf("delete object %q from bucket %q: %w", objectKey, s.bucket, err)
	}
	return nil
}

func (s BlobStore) deleteVersions(ctx context.Context, objectKey string) error {
	var latest time.Time
	retained := false
	paginator := s3.NewListObjectVersionsPaginator(s.client, &s3.ListObjectVersionsInput{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(objectKey),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list the versions of %q in bucket %q: %w", objectKey, s.bucket, err)
		}
		for i := range page.Versions {
			if aws.ToString(page.Versions[i].Key) != objectKey {
				continue
			}
			until, err := s.deleteVersion(ctx, objectKey, aws.ToString(page.Versions[i].VersionId))
			if err != nil {
				return err
			}
			if !until.IsZero() {
				retained = true
				latest = maxTime(latest, until)
			}
		}
		for _, marker := range page.DeleteMarkers {
			if aws.ToString(marker.Key) != objectKey || retained {
				continue
			}
			if _, err := s.deleteVersion(ctx, objectKey, aws.ToString(marker.VersionId)); err != nil {
				return err
			}
		}
	}
	if retained {
		return repository.NewRetainedError(latest)
	}
	return nil
}

func (s BlobStore) deleteVersion(ctx context.Context, objectKey, versionID string) (time.Time, error) {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket:    aws.String(s.bucket),
		Key:       aws.String(objectKey),
		VersionId: aws.String(versionID),
	})
	if err == nil || isNotFoundError(err) {
		return time.Time{}, nil
	}
	if !isAccessDenied(err) {
		return time.Time{}, fmt.Errorf("delete version %q of %q from bucket %q: %w", versionID, objectKey, s.bucket, err)
	}
	head, headErr := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket:    aws.String(s.bucket),
		Key:       aws.String(objectKey),
		VersionId: aws.String(versionID),
	})
	if headErr != nil || head.ObjectLockRetainUntilDate == nil || !head.ObjectLockRetainUntilDate.After(time.Now()) {
		return time.Time{}, fmt.Errorf("delete version %q of %q from bucket %q: %w", versionID, objectKey, s.bucket, err)
	}
	return *head.ObjectLockRetainUntilDate, nil
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func objectLockMode(mode repository.ObjectLockMode) string {
	if mode == repository.ComplianceObjectLock {
		return string(types.ObjectLockModeCompliance)
	}
	return string(types.ObjectLockModeGovernance)
}

func isObjectLockNotConfigured(err error) bool {
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		return apiErr.ErrorCode() == "ObjectLockConfigurationNotFoundError"
	}
	return false
}

func isAccessDenied(err error) bool {
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		return apiErr.ErrorCode() == "AccessDenied"
	}
	return false
}
