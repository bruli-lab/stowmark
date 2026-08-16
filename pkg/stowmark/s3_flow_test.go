//go:build integration

package stowmark_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestS3RepositoryFlow(t *testing.T) {
	for k, version := range Versions() {
		t.Run(`Running S3 workflow, with format version `+k, func(t *testing.T) {
			ctx := t.Context()

			s3Container := startS3Container(t, ctx)
			endpoint := s3ContainerEndpoint(t, ctx, s3Container)

			t.Setenv("AWS_ACCESS_KEY", "stowmark")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "stowmark-secret")
			t.Setenv("AWS_REGION", "us-east-1")
			t.Setenv("STOWMARK_S3_ENDPOINT", endpoint)
			t.Setenv("STOWMARK_S3_PATH_STYLE", "true")

			createS3Bucket(t, ctx, endpoint, "stowmark")

			repositoryURL := "s3://stowmark/backups"

			testRoot := t.TempDir()
			sourcePath := filepath.Join(testRoot, "source")
			restorePath := filepath.Join(testRoot, "restore")

			require.NoError(t, os.MkdirAll(sourcePath, 0o755))
			createSourceFixture(t, sourcePath)

			mainFlow(t, ctx, Folders{
				Source:     sourcePath,
				Repository: repositoryURL,
				Restore:    &restorePath,
			}, version)
		})
	}
}
