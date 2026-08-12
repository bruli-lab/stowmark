package smb

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

const DefaultSMBPort = "445"

type RepositoryURL struct {
	Address   string
	Username  string
	ShareName string
	BasePath  string
}

func ParseRepositoryURL(rawURL string) (*RepositoryURL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse SMB URL: %w", err)
	}

	if parsed.Scheme != "smb" {
		return nil, fmt.Errorf("invalid scheme %q: expected smb", parsed.Scheme)
	}

	if parsed.Hostname() == "" {
		return nil, fmt.Errorf("SMB host is required")
	}

	segments := strings.Split(
		strings.Trim(parsed.EscapedPath(), "/"),
		"/",
	)
	if len(segments) == 0 || segments[0] == "" {
		return nil, fmt.Errorf("SMB share name is required")
	}

	shareName, err := url.PathUnescape(segments[0])
	if err != nil {
		return nil, fmt.Errorf("decode SMB share name: %w", err)
	}

	var basePath string
	if len(segments) > 1 {
		decodedSegments := make([]string, 0, len(segments)-1)

		for _, segment := range segments[1:] {
			decoded, err := url.PathUnescape(segment)
			if err != nil {
				return nil, fmt.Errorf("decode SMB path: %w", err)
			}

			decodedSegments = append(decodedSegments, decoded)
		}

		basePath = strings.Join(decodedSegments, "/")
	}

	port := parsed.Port()
	if port == "" {
		port = DefaultSMBPort
	}

	username := ""
	if parsed.User != nil {
		username = parsed.User.Username()
	}

	return &RepositoryURL{
		Address:   net.JoinHostPort(parsed.Hostname(), port),
		Username:  username,
		ShareName: shareName,
		BasePath:  basePath,
	}, nil
}
