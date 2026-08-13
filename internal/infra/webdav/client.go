package webdav

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/studio-b12/gowebdav"
)

type Location struct {
	BaseURL string
	Path    string
}

func ParseWebDAVURL(repositoryURL string) (*Location, error) {
	u, err := url.Parse(repositoryURL)
	if err != nil {
		return nil, fmt.Errorf("parse WebDAV URL: %w", err)
	}

	switch strings.ToLower(u.Scheme) {
	case "webdav":
		u.Scheme = "http"
	case "webdavs":
		u.Scheme = "https"
	default:
		return nil, fmt.Errorf(
			"unsupported WebDAV scheme %q",
			u.Scheme,
		)
	}

	if u.Host == "" {
		return nil, fmt.Errorf("WebDAV URL requires a host")
	}

	repositoryPath := strings.Trim(u.Path, "/")

	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""

	return &Location{
		BaseURL: strings.TrimRight(u.String(), "/"),
		Path:    repositoryPath,
	}, nil
}

func NewClient(baseURL, username, password string) (*gowebdav.Client, error) {
	client := gowebdav.NewClient(baseURL, username, password)
	client.SetTimeout(30 * time.Second)

	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("connect to WebDAV server %q: %w", baseURL, err)
	}

	return client, nil
}
