//go:build integration

package stowmark_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/bruli-lab/stowmark/pkg/stowmark"
	"github.com/stretchr/testify/require"
)

const (
	objectLockPeriod = 4 * time.Second
	orphanObjectKey  = "backups/objects/ab/cdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
)

func TestS3ObjectLockFlow(t *testing.T) {
	ctx := t.Context()

	s3Container := startS3Container(t, ctx)
	endpoint := s3ContainerEndpoint(t, ctx, s3Container)

	t.Setenv("AWS_ACCESS_KEY_ID", "stowmark")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "stowmark-secret")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("STOWMARK_S3_ENDPOINT", endpoint)
	t.Setenv("STOWMARK_S3_PATH_STYLE", "true")

	lock := &stowmark.ObjectLock{Mode: stowmark.GovernanceObjectLock, Period: objectLockPeriod}

	t.Run(`Given a bucket without object lock,
	when a repository is initialized with object lock,
	then it fails before writing anything`, func(t *testing.T) {
		createS3Bucket(t, ctx, endpoint, "plain")
		handler := openHandler(t, ctx, "s3://plain/backups")

		err := handler.InitWithOptions(ctx, stowmark.InitOptions{ObjectLock: lock})

		require.ErrorContains(t, err, "object lock")
	})

	client := createObjectLockS3Bucket(t, ctx, endpoint, "locked")
	repositoryURL := "s3://locked/backups"
	source := filepath.Join(t.TempDir(), "source")
	require.NoError(t, os.MkdirAll(source, 0o750))
	writeSource := func(content string) {
		require.NoError(t, os.WriteFile(filepath.Join(source, "file.txt"), []byte(content), 0o600))
	}

	handler := openHandler(t, ctx, repositoryURL)
	require.NoError(t, handler.InitWithOptions(ctx, stowmark.InitOptions{FormatVersion: 3, ObjectLock: lock}))

	writeSource("first version")
	_, err := handler.CreateSnapshot(ctx, source, nil)
	require.NoError(t, err)
	writeSource("second version")
	second, err := handler.CreateSnapshot(ctx, source, nil)
	require.NoError(t, err)

	t.Run(`Given snapshots still under retention,
	when forget does not keep the oldest one,
	then it postpones it instead of deleting it`, func(t *testing.T) {
		result, err := handler.ForgetSnapshots(ctx, stowmark.RetentionPolicy{KeepLast: 1})

		require.NoError(t, err)
		require.Empty(t, result.Forgotten)
		require.Len(t, result.Postponed, 1)
		require.True(t, result.Postponed[0].RetainedUntil.After(time.Now()))
		listed, err := handler.ListSnapshots(ctx)
		require.NoError(t, err)
		require.Len(t, listed, 2)
	})

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:                    aws.String("locked"),
		Key:                       aws.String(orphanObjectKey),
		Body:                      strings.NewReader("left by an interrupted write"),
		ObjectLockMode:            types.ObjectLockModeGovernance,
		ObjectLockRetainUntilDate: aws.Time(time.Now().Add(time.Hour)),
		ChecksumAlgorithm:         types.ChecksumAlgorithmCrc32,
	})
	require.NoError(t, err)

	t.Run(`Given an orphan object under retention,
	when prune is run,
	then it postpones its deletion and the repository is still healthy`, func(t *testing.T) {
		result, err := handler.Prune(ctx)

		require.NoError(t, err)
		require.Zero(t, result.RemovedObjects)
		require.Equal(t, 1, result.PostponedObjects)
		require.True(t, result.RetainedUntil.After(time.Now().Add(50*time.Minute)))
		check, err := handler.Check(ctx, stowmark.CheckOptions{})
		require.NoError(t, err)
		require.False(t, check.HasErrors)
		require.Zero(t, check.UnprotectedItems)
	})

	time.Sleep(objectLockPeriod + time.Second)

	t.Run(`Given a repository whose retention has ended,
	when check and retention extend are run,
	then check warns about the unprotected items and extend protects them again`, func(t *testing.T) {
		before, err := handler.Check(ctx, stowmark.CheckOptions{})
		require.NoError(t, err)
		require.Equal(t, 1+2+2, before.UnprotectedItems)

		extended, err := handler.ExtendRetention(ctx)
		require.NoError(t, err)
		require.Equal(t, 1+2+2, extended.Extended)
		require.True(t, extended.RetainedUntil.After(time.Now()))

		after, err := handler.Check(ctx, stowmark.CheckOptions{})
		require.NoError(t, err)
		require.Zero(t, after.UnprotectedItems)

		again, err := handler.ExtendRetention(ctx)
		require.NoError(t, err)
		require.Zero(t, again.Extended)
	})

	time.Sleep(objectLockPeriod + time.Second)

	t.Run(`Given a repository whose retention has ended again,
	when forget and prune are run,
	then they delete the old snapshot and its object, keep the retained orphan and the last snapshot still restores`, func(t *testing.T) {
		forgotten, err := handler.ForgetSnapshots(ctx, stowmark.RetentionPolicy{KeepLast: 1})
		require.NoError(t, err)
		require.Len(t, forgotten.Forgotten, 1)
		require.Empty(t, forgotten.Postponed)

		pruned, err := handler.Prune(ctx)
		require.NoError(t, err)
		require.Equal(t, 1, pruned.RemovedObjects)
		require.Equal(t, 1, pruned.PostponedObjects)

		destination := filepath.Join(t.TempDir(), "restore")
		restored, err := handler.RestoreSnapshot(ctx, second.ID, &destination, nil)
		require.NoError(t, err)
		require.True(t, restored.IsSuccess)
		content, err := os.ReadFile(filepath.Join(destination, "file.txt"))
		require.NoError(t, err)
		require.Equal(t, "second version", string(content))

		check, err := handler.Check(ctx, stowmark.CheckOptions{ReadData: true})
		require.NoError(t, err)
		require.False(t, check.HasErrors)
	})
}
