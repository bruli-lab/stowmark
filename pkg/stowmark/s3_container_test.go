//go:build integration

package stowmark_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	testcontainerslog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startS3Container(t *testing.T, ctx context.Context) testcontainers.Container {
	t.Helper()

	s3Container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "motoserver/moto:5.2.2",
			ExposedPorts: []string{"5000/tcp"},
			Env: map[string]string{
				"MOTO_SERVICE": "s3",
			},
			WaitingFor: wait.ForHTTP("/moto-api/").
				WithPort("5000/tcp").
				WithStartupTimeout(60 * time.Second),
		},
		Started: true,
		Logger:  testcontainerslog.TestLogger(t),
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, testcontainers.TerminateContainer(s3Container))
	})

	return s3Container
}

func s3ContainerEndpoint(t *testing.T, ctx context.Context, s3Container testcontainers.Container) string {
	t.Helper()

	host, err := s3Container.Host(ctx)
	require.NoError(t, err)

	mappedPort, err := s3Container.MappedPort(ctx, "5000/tcp")
	require.NoError(t, err)

	return "http://" + net.JoinHostPort(host, mappedPort.Port())
}

func createS3Bucket(t *testing.T, ctx context.Context, endpoint, bucket string) {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			"stowmark",
			"stowmark-secret",
			"",
		)),
	)
	require.NoError(t, err)

	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})

	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	require.NoError(t, err)
}
