package s3

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type Storage struct {
	Bucket, Prefix string
}

func ParseS3URL(rawURL string) (storage *Storage, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse S3 URL: %w", err)
	}

	if u.Scheme != "s3" {
		return nil, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}

	if u.Host == "" {
		return nil, errors.New("S3 bucket is required")
	}

	bucket := u.Host
	prefix := strings.Trim(u.Path, "/")

	return &Storage{
		Bucket: bucket,
		Prefix: prefix,
	}, nil
}
