package gcs

import (
	"errors"
	"fmt"
	"net/url"
)

func ParseBucket(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse GCS URL: %w", err)
	}

	if u.Scheme != "gcs" {
		return "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}

	if u.Host == "" {
		return "", errors.New("GCS bucket is required")
	}

	return u.Host, nil
}
